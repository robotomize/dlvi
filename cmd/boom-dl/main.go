package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/robotomize/dlvi/pkg/sizedbuf"
)

const defaultFlushLimit = 512 * 1024

var (
	chunkList string
	storePth  string
	xorKey    string
	cipherKey string
)

var httpClient = http.DefaultClient

func init() {
	flag.StringVar(&chunkList, "p", "", "-p http://chunklist.m3u8")
	flag.StringVar(&storePth, "s", "./_video", "-s path/to/videos")
	flag.StringVar(&xorKey, "x", "bla_bla_bla", "-x bla_bla_bla")
	flag.StringVar(&cipherKey, "c", "cipher", "-c cipherkeyvalue")
	flag.Parse()
}

var (
	readyKey string
	hexKey   string
	iv       string
)

func main() {
	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := os.MkdirAll(storePth, 0755); err != nil {
		if !errors.Is(err, os.ErrExist) {
			log.Fatal(err)
		}
	}

	client := http.DefaultClient
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chunkList, nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	chunkCh := make(chan string)

	wg := sync.WaitGroup{}
	wg.Add(runtime.NumCPU())
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			defer wg.Done()
			for ch := range chunkCh {
				if err := downloadSegment(ctx, ch); err != nil {
					log.Fatal(err)
				}

				fmt.Println("Downloaded segment:", ch)
			}
		}()
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			log.Fatal(ctx.Err())
		default:
		}

		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA-READY"):
			if idx := strings.Index(line, ":"); idx != -1 {
				readyKey = line[idx+1:]
				decr, err := decodeReadyKey(readyKey, xorKey)
				if err != nil {
					log.Fatal(err)
				}

				iv = hex.EncodeToString([]byte(decr[20:36]))
				hexKey = hex.EncodeToString([]byte(cipherKey))
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			scanner.Scan()
			chunkURL := scanner.Text()
			chunkCh <- chunkURL
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Println("Error scanning file:", err)
	}
	close(chunkCh)

	wg.Wait()

	if err := mergeSegments(ctx); err != nil {
		log.Fatal(err)
	}

	if err := transcodeMP4(); err != nil {
		log.Fatal(err)
	}

	dir, err := os.ReadDir(storePth)
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range dir {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ts") {
			pth := filepath.Join(storePth, entry.Name())
			if err := os.Remove(pth); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func transcodeMP4() error {
	decrPth := filepath.Join(storePth, "output.ts")
	decodedPth := filepath.Join(storePth, "decoded.mp4")
	cmd := exec.Command("ffmpeg", "-i", decrPth, "-c", "copy", decodedPth)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("exec Command Run ffmpeg: %w", err)
	}

	return nil
}

func mergeSegments(ctx context.Context) error {
	dir, err := os.ReadDir(storePth)
	if err != nil {
		return fmt.Errorf("os ReadDir: %w", err)
	}

	var n int
	for idx, d := range dir {
		if !d.IsDir() {
			dir[n] = dir[idx]
			n++
		}
	}
	dir = dir[:n]

	sort.Slice(
		dir, func(i, j int) bool {
			vi := strings.TrimPrefix(dir[i].Name(), "media-")
			vi = strings.TrimSuffix(vi, ".ts")
			vid, err := strconv.Atoi(vi)
			if err != nil {
				return false
			}

			vj := strings.TrimPrefix(dir[j].Name(), "media-")
			vj = strings.TrimSuffix(vj, ".ts")
			vjd, err := strconv.Atoi(vj)
			if err != nil {
				return false
			}

			return vid < vjd
		},
	)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	f, err := os.Create(filepath.Join(storePth, "output.ts"))
	if err != nil {
		return fmt.Errorf("os Create: %w", err)
	}
	defer f.Close()

	buf := sizedbuf.New(f, defaultFlushLimit)

	for _, entry := range dir {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pth := filepath.Join(storePth, entry.Name())
		if err := copyFile(buf, pth); err != nil {
			return fmt.Errorf("copyFile: %w", err)
		}
	}

	if err := buf.Flush(); err != nil {
		return fmt.Errorf("sizedbuf Flush: %w", err)
	}

	return nil
}

func copyFile(w io.Writer, pth string) error {
	f, err := os.Open(pth)
	if err != nil {
		return fmt.Errorf("os Open: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("io Copy: %w", err)
	}

	return nil
}

func downloadSegment(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("http.NewRequest")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http client Do: %w", err)
	}
	defer resp.Body.Close()

	pth := filepath.Join(storePth, getFileNameFromURL(url))
	f, err := os.Create(pth)
	if err != nil {
		return fmt.Errorf("os Create: %w", err)
	}
	defer f.Close()

	buf := sizedbuf.New(f, defaultFlushLimit)
	cmd := exec.Command("openssl", "aes-128-cbc", "-K", hexKey, "-iv", iv, "-d")
	cmd.Stdin = resp.Body
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("exec Command Run openssl: %w", err)
	}

	if err := buf.Flush(); err != nil {
		return fmt.Errorf("sizedbuf Flush: %w", err)
	}

	return nil
}

func getFileNameFromURL(url string) string {
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

func decodeReadyKey(source string, key string) (string, error) {
	for len(key) < len(source) {
		key += key
	}

	result := make([]rune, 0, len(source))
	for n := 0; n < len(source); n += 2 {
		parseInt, err := strconv.ParseInt(source[n:n+2], 16, 64)
		if err != nil {
			return "", err
		}

		c := int32(parseInt) ^ int32(key[n/2])
		result = append(result, c)
	}

	return string(result), nil
}

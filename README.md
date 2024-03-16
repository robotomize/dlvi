# Boomstream downloader
This Go project is designed to download and transcode video files from HLS (HTTP Live Streaming) into MP4 format. The program downloads video segments from an HLS playlist, decrypts them if necessary, merges them into a single file, and then transcodes the merged file into MP4 using ffmpeg.

## Why?
You should only use this for research purposes. To download the boomstream video.

## Features
* Parallel Downloads: Utilizes all available CPU cores to download video segments in parallel, speeding up the download process.
* Decryption: Supports decryption of HLS segments using AES-128-CBC cipher with a provided key and initialization vector (IV).
* Transcoding: Transcodes the merged .ts file into an MP4 file using ffmpeg.

## Usage
To use this program, you need to provide the HLS playlist URL, the path to store the downloaded files, and the decryption key if the segments are encrypted.
```shell
go run main.go -p http://chunklist.m3u8 -s path/to/videos -x bla_bla_bla -c cipherkeyvalue
```

## Dependencies
* ffmpeg: Required for transcoding the merged .ts file into MP4.
* openssl: Required for decrypting the HLS segments if they are encrypted.
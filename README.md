# image-fs Windows

Experimental Windows port of image-fs.

This project allows a FAT32 partition to be stored inside PNG/JPG images and mounted as a virtual drive on Windows.

## About

image-fs Windows is a Go-based CLI application adapted from Xelckis' image-fs.

The program reads the image structure, locates the end of the image data, and accesses the hidden FAT32 partition stored after the image.

## Features

- Hide FAT32 partitions inside PNG/JPG files
- Detect embedded partitions
- Mount hidden partitions on Windows
- Open mounted drives automatically in Explorer

## Requirements

- Windows 10/11
- Go 1.25+ (for compiling)
- OSFMount

## Usage

### Inject

```cmd
main.exe inject -image image.png -part fat32.img
```

### Mount

```cmd
main.exe mount -image image.png
```

## Credits

Based on @Xelckis's image-fs:
https://github.com/Xelckis/image-fs

## License

Apache License 2.0

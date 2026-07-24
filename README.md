# image-fs: Image Partition Injector and Mounter

## About the Project
This project is a Go-based Command Line Interface (CLI) application designed for steganography and virtual disk management. It allows users to hide a FAT32 partition inside an image file (specifically JPG or PNG) and seamlessly mount that hidden partition directly to the file system.

The tool works by parsing the hexadecimal structure of the target image to find the "End of Image" (EoI) markers (`0xFF 0xD9` for JPG and `IEND` for PNG). It calculates this offset and securely appends the FAT32 partition data right after the image data ends, ensuring the image remains viewable while carrying a hidden payload.

## Requirements
To compile and run this project, you will need the following:
* **Go Environment:** The project is configured for Go version 1.25.0.
* **Linux Operating System:** The mounting feature heavily relies on the Linux `udisks2` service.
* **udisksctl:** The `udisksctl` command must be installed and available in your system's PATH, as the application executes it to setup loop devices and mount the file system.
* **Target Files:** A valid `.jpg` or `.png` image file, and a pre-formatted FAT32 partition file (like the `disk.fat32` file included in the project structure).

## How to Use

First, compile the Go code into an executable (e.g., `file-fs`). The application provides two main commands: `inject` and `mount`.

### 1. Injecting a Partition
To hide a FAT32 partition inside an image, use the `inject` command. You must provide the path to the target image and the path to the partition file.

**Command:**
```bash
./file-fs inject -image <path_to_image> -part <path_to_fat32_partition>
```

*Example:*

```bash
./file-fs inject -image images.jpg -part disk.fat32

```

**What happens:** The program will read the image structure, confirm the PNG or JPG signature, calculate the exact byte offset where the image data ends, and append the partition data to the image file.

### 2. Mounting the Hidden Partition

To access the hidden files, use the `mount` command to attach the injected image as a virtual disk.

**Command:**

```bash
./file-fs mount -image <path_to_image>

```

*Example:*

```bash
./file-fs mount -image images.jpg

```

**What happens:**

* The program locates the hidden partition within the image.


* It connects the virtual disk to the motherboard using `udisksctl loop-setup` with the calculated offset.


* It waits for `udev` to analyze the signature and then mounts the partition on the file system.


* Once successful, the disk will appear in your file manager's sidebar.



### 3. Unmounting Safely

While the disk is mounted, the application will remain running in the terminal. To safely unmount the partition:

* Press `[CTRL+C]` in the terminal running the application.


* The program will intercept the signal and automatically execute the unmount and loop-delete commands to eject the virtual hardware and erase session traces.



## License

This software is distributed under the Apache License, Version 2.0.

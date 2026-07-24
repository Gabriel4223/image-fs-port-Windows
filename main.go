package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var (
	MagicJPG = []byte{0xFF, 0xD8, 0xFF}
	MagicPNG = []byte{0x89, 0x50, 0x4E, 0x47}
	EoIJPG   = []byte{0xFF, 0xD9}
	EoIPNG   = []byte("IEND")
)

func MountNativeDrive(targetImage string, offset int) error {
	alignedOffset := offset
	if offset%512 != 0 {
		alignedOffset = ((offset / 512) + 1) * 512
	}

	log.Printf("[*] Conectando disco virtual ao Windows (Offset original: %d, Alinhado: %d)...\n", offset, alignedOffset)

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("falha ao identificar o diretorio do executavel: %v", err)
	}
	projectRoot := filepath.Dir(exePath)
	osfmountTool := filepath.Join(projectRoot, "tools", "osfmount", "OSFMount.com")

	if _, err := os.Stat(osfmountTool); os.IsNotExist(err) {
		return fmt.Errorf("ferramenta nao encontrada em: %s", osfmountTool)
	}

	absTargetImage, err := filepath.Abs(targetImage)
	if err != nil {
		return fmt.Errorf("falha ao obter o caminho absoluto da imagem: %v", err)
	}

	offsetStr := fmt.Sprintf("%d", alignedOffset)
	setupCmd := exec.Command(osfmountTool, "-a", "-t", "file", "-f", absTargetImage, "-b", offsetStr, "-m", "#:", "-o", "rw")

	output, err := setupCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("falha ao criar o disco virtual (OSFMount): %s - %v", string(output), err)
	}

	re := regexp.MustCompile(`([A-Z]:|[A-Z]\b)`)
	match := re.FindString(string(output))

	driveLetter := ""
	if match != "" {
		driveLetter = strings.ToUpper(match)
		if len(driveLetter) == 1 {
			driveLetter += ":"
		}
		log.Printf("[+] Hardware virtual reconhecido no Windows: %s\n", driveLetter)
		
		log.Printf("[*] Abrindo a unidade %s no Explorador de Arquivos...\n", driveLetter)
		go func() {
			time.Sleep(800 * time.Millisecond)
			exec.Command("explorer.exe", driveLetter).Run()
		}()
	} else {
		log.Println("[!] Nao foi possivel ler a letra no retorno. Abrindo 'Meu Computador'...")
		go exec.Command("explorer.exe", "shell:MyComputerFolder").Run()
	}

	log.Println("[+] Disco montado com sucesso!")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	fmt.Println("\n=======================================================")
	fmt.Println(" -> Mantenha esta janela aberta para usar o disco.     ")
	fmt.Println(" -> Pressione [CTRL+C] aqui para ejetar com segurança.  ")
	fmt.Println("=======================================================")

	<-sigChan

	log.Println("[!] Interrupcao detectada. Acionando painel de confirmacao nativo...")

	// CORREÇÃO ESTRATÉGICA: Aponta para a versão GUI (.exe em vez de .com) para forçar o Windows a exibir o Pop-up
	osfmountGUI := filepath.Join(projectRoot, "tools", "osfmount", "OSFMount.exe")

	// 1. Fecha janelas do Windows Explorer que estejam olhando para o IMAGEFS para não travar o buffer
	exec.Command("taskkill", "/IM", "explorer.exe", "/FI", "WINDOWTITLE eq IMAGEFS*").Run()
	time.Sleep(300 * time.Millisecond)

	// 2. Chama a ejeção forçada pela Letra utilizando o executável gráfico
	// Isso faz o Windows exibir a janelinha de confirmação "Do you wish to continue?" na tela
	if driveLetter != "" {
		log.Printf("[*] Solicitando desmonte da unidade %s via painel nativo...\n", driveLetter)
		exec.Command(osfmountGUI, "-D", "-m", driveLetter).Run()
	} else {
		// Margem de segurança caso não tenha lido a letra
		exec.Command(osfmountGUI, "-D", "-m", "L:").Run()
	}

	// 3. Limpeza física de contingência sobre o arquivo original
	exec.Command(osfmountTool, "-d", "-f", absTargetImage).Run()

	log.Println("[+] Processo de desmontagem finalizado. Sessao encerrada.")
	return nil
}

func injectPartition(f *os.File, partitionPath string, startByte int) error {
	_, err := f.Seek(int64(startByte), io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to set file pointer: %v", err)
	}

	paddingNeeded := 0
	if startByte%512 != 0 {
		paddingNeeded = 512 - (startByte % 512)
	}

	if paddingNeeded > 0 {
		log.Printf("[*] Adicionando %d bytes de preenchimento para alinhar com o kernel Windows...\n", paddingNeeded)
		padding := make([]byte, paddingNeeded)
		_, err = f.Write(padding)
		if err != nil {
			return fmt.Errorf("failed to write sector padding bytes: %v", err)
		}
	}

	p, err := os.Open(partitionPath)
	if err != nil {
		return fmt.Errorf("failed to open partition file: %v", err)
	}
	defer p.Close()

	buffer := make([]byte, 4096)
	for {
		_, err := p.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error during partition file read: %v", err)
		}

		_, err = f.Write(buffer)
		if err != nil {
			return fmt.Errorf("error writing partition to image file: %v", err)
		}
	}

	return nil
}

func identifyLastMarker(f *os.File, marker []byte) (int, error) {
	_, err := f.Seek(0, io.SeekStart)
	if err != nil {
		return 0, fmt.Errorf("failed to reset file pointer: %v", err)
	}

	buffer := make([]byte, 4096)
	window := make([]byte, 0, 8192)

	var absoluteOffset int = 0
	var lastMarkerPos int = -1
	overlap := len(marker) - 1

	for {
		n, err := f.Read(buffer)
		if n > 0 {
			window = append(window, buffer[:n]...)

			if idx := bytes.LastIndex(window, marker); idx != -1 {
				lastMarkerPos = absoluteOffset + idx
			}

			if len(window) > overlap {
				absoluteOffset += len(window) - overlap
				window = append([]byte(nil), window[len(window)-overlap:]...)
			} else {
				absoluteOffset += len(window)
				window = window[:0]
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("error during file read: %v", err)
		}
	}

	if lastMarkerPos != -1 {
		return lastMarkerPos + len(marker), nil
	}

	return 0, fmt.Errorf("end-of-image marker not found in file")
}

func ParseImage(filePath string) (int, string, error) {
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return 0, "", fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	header := make([]byte, 8)
	_, err = file.Read(header)
	if err != nil && err != io.EOF {
		return 0, "", fmt.Errorf("failed to read file header: %v", err)
	}

	if bytes.HasPrefix(header, MagicPNG) {
		pos, err := identifyLastMarker(file, EoIPNG)
		if err != nil {
			return 0, "", err
		}
		return pos + 4, "PNG", nil

	} else if bytes.HasPrefix(header, MagicJPG) {
		pos, err := identifyLastMarker(file, EoIJPG)
		if err != nil {
			return 0, "", err
		}
		return pos, "JPG", nil
	}

	return 0, "", fmt.Errorf("magic bytes do not match a PNG or JPG file")
}

func main() {
	injectCmd := flag.NewFlagSet("inject", flag.ExitOnError)
	injectImage := injectCmd.String("image", "", "Path to the target image (required)")
	injectPart := injectCmd.String("part", "", "Path to the fat32 partition file (required)")

	mountCmd := flag.NewFlagSet("mount", flag.ExitOnError)
	mountImage := mountCmd.String("image", "", "Path to the injected image (required)")

	if len(os.Args) < 2 {
		fmt.Println("Usage: program <command> [flags]")
		fmt.Println("Available commands:")
		fmt.Println("  inject  - Injects a fat32 partition into an image")
		fmt.Println("  mount   - Mounts the hidden partition from the image via OSFMount")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "inject":
		injectCmd.Parse(os.Args[2:])
		if *injectImage == "" || *injectPart == "" {
			fmt.Println("[-] Error: The -image and -part flags are required.")
			injectCmd.PrintDefaults()
			os.Exit(1)
		}

		log.Printf("[*] Starting hexadecimal dissection of file: %s\n", *injectImage)

		offset, fileType, err := ParseImage(*injectImage)
		if err != nil {
			log.Fatalf("[-] Fatal operation failure: %v\n", err)
		}

		fmt.Printf("[+] Signature confirmed: %s\n", fileType)
		fmt.Printf("[+] Offset calculated successfully: byte %d\n", offset)
		fmt.Printf("[!] From position %d, memory is clear for fat32 partition injection.\n", offset)
		fmt.Printf("[+] Writing partition to image file...\n")

		file, err := os.OpenFile(*injectImage, os.O_RDWR, 0644)
		if err != nil {
			log.Fatalf("[-] Failed to open image file: %v\n", err)
		}
		defer file.Close()

		err = injectPartition(file, *injectPart, offset)
		if err != nil {
			fmt.Printf("[-] Injection failed: %v\n", err)
			return
		}

		fmt.Printf("[+] Partition wrote to image file :)\n")

	case "mount":
		mountCmd.Parse(os.Args[2:])
		if *mountImage == "" {
			fmt.Println("[-] Error: The -image flag is required.")
			mountCmd.PrintDefaults()
			os.Exit(1)
		}

		log.Printf("[*] Reading image structure to locate partition: %s\n", *mountImage)

		offset, _, err := ParseImage(*mountImage)
		if err != nil {
			log.Fatalf("[-] Fatal operation failure: %v\n", err)
		}

		err = MountNativeDrive(*mountImage, offset)
		if err != nil {
			fmt.Printf("[-] Mount failed: %v\n", err)
			return
		}

	default:
		fmt.Printf("[-] Unknown command: %s\n", os.Args[1])
		fmt.Println("Available commands: inject, mount")
		os.Exit(1)
	}
}

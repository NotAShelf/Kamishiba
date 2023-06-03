package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	searchURL       = "https://m.manganelo.com/search/story/"
	defaultImageDir = ".cache/manga-cli/"
)

var (
	// Colors
	greenColor  = "\033[0;32m"
	yellowColor = "\033[0;33m"
	whiteColor  = "\033[0;37m"

	// Commands
	lsCommand  = "ls"
	zipCommand = "zip"
	zathuraCmd = "zathura"
)

func main() {
	checkDependencies()
	searchManga()
}

func checkDependencies() {
	commands := []string{lsCommand, zipCommand, zathuraCmd}

	for _, cmd := range commands {
		_, err := exec.LookPath(cmd)
		if err != nil {
			log.Fatalf("Error: %s command not found.", cmd)
		}
	}
}

func searchManga() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(yellowColor + "Search manga: ")
	mangaName, _ := reader.ReadString('\n')
	mangaName = strings.TrimSpace(mangaName)

	mangaName = strings.ReplaceAll(mangaName, " ", "_")
	mangaName = strings.ReplaceAll(mangaName, "-", "_")

	mangaURL := searchURL + mangaName

	resp, err := http.Get(mangaURL)
	if err != nil {
		log.Fatal("Failed to search manga:", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Failed to read search response:", err)
	}

	mangaNames := extractTextBySelector(string(body), "div.item-right h3 a")
	mangaIDs := extractAttrBySelector(string(body), "div.item-right h3 a", "href")

	if len(mangaNames) == 0 {
		log.Fatal("No manga found for the given search terms")
	}

	fmt.Println(whiteColor + "Search results:")
	for i, name := range mangaNames {
		fmt.Printf("[%d] %s\n", i+1, name)
	}

	fmt.Print(yellowColor + "Enter Number: ")
	mangaNumberStr, _ := reader.ReadString('\n')
	mangaNumberStr = strings.TrimSpace(mangaNumberStr)

	mangaNumber, err := strconv.Atoi(mangaNumberStr)
	if err != nil || mangaNumber < 1 || mangaNumber > len(mangaIDs) {
		log.Fatal("Invalid manga number")
	}

	mangaName = mangaNames[mangaNumber-1]
	mangaName = strings.ReplaceAll(mangaName, " ", "_")
	mangaName = normalizeString(mangaName)

	mangaLink := mangaIDs[mangaNumber-1]

	selectChapter(mangaName, mangaLink)
}

func selectChapter(mangaName, mangaLink string) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(yellowColor + "Enter chapter number: ")
	chapterNumberStr, _ := reader.ReadString('\n')
	chapterNumberStr = strings.TrimSpace(chapterNumberStr)

	chapterNumber, err := strconv.Atoi(chapterNumberStr)
	if err != nil || chapterNumber < 1 {
		log.Fatal("Invalid chapter number")
	}

	getImages(mangaName, mangaLink, chapterNumber)
}

func getImages(mangaName, mangaLink string, chapterNumber int) {
	imageDir := filepath.Join(getHomeDir(), defaultImageDir)
	chapterLink := mangaLink + "/chapter-" + strconv.Itoa(chapterNumber)

	if fileExists(filepath.Join(imageDir, fmt.Sprintf("%s-%d.cbz", mangaName, chapterNumber))) {
		fmt.Println(greenColor + "Manga file exists in cache, opening it...")
		openFile(filepath.Join(imageDir, fmt.Sprintf("%s-%d.cbz", mangaName, chapterNumber)))
		chooseNext(mangaName, mangaLink, chapterNumber)
		return
	}

	html, err := fetchHTML(chapterLink)
	if err != nil {
		log.Fatal("Failed to fetch chapter HTML:", err)
	}

	imageLinks := extractAttrBySelector(html, "div.container-chapter-reader img", "src")

	if len(imageLinks) == 0 {
		log.Fatal("No images found for the chapter")
	}

	err = createFile(imageLinks, imageDir, mangaName, chapterNumber)
	if err != nil {
		log.Fatal("Failed to create manga file:", err)
	}
}

func fetchHTML(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func createFile(imageLinks []string, imageDir, mangaName string, chapterNumber int) error {
	if err := os.MkdirAll(imageDir, os.ModePerm); err != nil {
		return err
	}

	fmt.Println(greenColor + "Downloading images...")

	for i, link := range imageLinks {
		resp, err := http.Get(link)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		imagePath := filepath.Join(imageDir, fmt.Sprintf("%s-%d-%d.jpg", mangaName, chapterNumber, i+1))

		file, err := os.Create(imagePath)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return err
		}
	}

	fmt.Println(greenColor + "Creating manga file...")

	err := createMangaFile(imageLinks, imageDir, mangaName, chapterNumber)
	if err != nil {
		return err
	}

	openFile(filepath.Join(imageDir, fmt.Sprintf("%s-%d.cbz", mangaName, chapterNumber)))

	return nil
}

func createMangaFile(imageLinks []string, imageDir, mangaName string, chapterNumber int) error {
	zipPath := filepath.Join(imageDir, fmt.Sprintf("%s-%d.cbz", mangaName, chapterNumber))
	zipCmd := exec.Command(zipCommand, "-q", zipPath)
	zipCmd.Dir = imageDir

	for i := 1; i <= len(imageLinks); i++ {
		imagePath := filepath.Join(imageDir, fmt.Sprintf("%s-%d-%d.jpg", mangaName, chapterNumber, i))
		zipCmd.Args = append(zipCmd.Args, imagePath)
	}

	if err := zipCmd.Run(); err != nil {
		return err
	}

	fmt.Println(greenColor + "Manga file created successfully!")

	return nil
}

func openFile(filePath string) {
	openCmd := exec.Command(zathuraCmd, filePath)
	err := openCmd.Run()
	if err != nil {
		log.Fatal("Failed to open manga file:", err)
	}
}

func chooseNext(mangaName, mangaLink string, chapterNumber int) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println(whiteColor + "Next chapter (n)")
	fmt.Println(whiteColor + "Quit (q)")
	fmt.Println(whiteColor + "Previous chapter (p)")

	fmt.Print(yellowColor + "Enter Option: ")
	option, _ := reader.ReadString('\n')
	option = strings.TrimSpace(option)

	switch option {
	case "n":
		chapterNumber++
		getImages(mangaName, mangaLink, chapterNumber)
	case "p":
		chapterNumber--
		getImages(mangaName, mangaLink, chapterNumber)
	case "q":
		os.Exit(0)
	default:
		chooseNext(mangaName, mangaLink, chapterNumber)
	}
}

func extractTextBySelector(html, selector string) []string {
	re := regexp.MustCompile(`<` + selector + `[^>]*>(.*?)</` + selector + `>`)
	matches := re.FindAllStringSubmatch(html, -1)

	var texts []string
	for _, match := range matches {
		texts = append(texts, match[1])
	}

	return texts
}

func extractAttrBySelector(html, selector, attrName string) []string {
	re := regexp.MustCompile(`<` + selector + `[^>]*` + attrName + `=["'](.*?)["']`)
	matches := re.FindAllStringSubmatch(html, -1)

	var attrs []string
	for _, match := range matches {
		attrs = append(attrs, match[1])
	}

	return attrs
}

func normalizeString(str string) string {
	reg := regexp.MustCompile(`[^[:alnum:]]`)
	return reg.ReplaceAllString(str, "")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Failed to get user home directory:", err)
	}
	return home
}

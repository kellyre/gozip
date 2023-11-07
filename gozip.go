package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// extractZip extracts the contents of the zip file into a subdirectory.
func extractZip(zipPath string, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		if err == zip.ErrFormat {
			fmt.Printf("Error: %s is not a valid zip file and will be skipped.\n", zipPath)
			return nil // Return nil to continue processing other files
		}
		return err
	}
	defer r.Close()

	os.MkdirAll(destDir, 0755)

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)

		// Close the file without defer to handle the error
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

// worker is a goroutine that processes zip files from pathsChan and sends errors to errChan.
func worker(wg *sync.WaitGroup, pathsChan <-chan string, errChan chan<- error) {
	defer wg.Done()
	for path := range pathsChan {
		destDir := strings.TrimSuffix(path, ".zip")

		// Check if the directory exists
		if _, err := os.Stat(destDir); os.IsNotExist(err) {
			fmt.Printf("Extracting %s...\n", path)
			err := extractZip(path, destDir)
			if err != nil {
				errChan <- err
				return
			}
		} else {
			fmt.Printf("Directory %s already exists, skipping...\n", destDir)
		}
	}
}

func main() {
	// Parse command-line flags
	var numWorkers int
	flag.IntVar(&numWorkers, "n", 2, "number of worker threads")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: gozip -n <number_of_workers> <directory>")
		os.Exit(1)
	}

	startDir := flag.Arg(0)

	// Channels for passing paths and errors
	pathsChan := make(chan string, numWorkers)
	errChan := make(chan error, numWorkers)

	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(&wg, pathsChan, errChan)
	}

	// Walk the directory tree and send zip files to the workers
	go func() {
		filepath.Walk(startDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				errChan <- err
				return err
			}

			if !info.IsDir() && strings.HasSuffix(path, ".zip") {
				pathsChan <- path
			}
			return nil
		})
		close(pathsChan)
	}()

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Check for errors from workers
	for err := range errChan {
		if err != nil {
			fmt.Printf("An error occurred: %s\n", err)
			os.Exit(1)
		}
	}
}

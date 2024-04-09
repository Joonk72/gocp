package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/dustin/go-humanize"

	"errors"
	"log"
	"net/http"
	_ "net/http/pprof"
)

type Args struct {
	Source  string
	Target  string
	Threads uint
}

var filePool = sync.Pool{
	New: func() interface{} {
		return &os.File{}
	},
}

type ThreadPool struct {
	tasks chan func()
	wg    sync.WaitGroup
}

func main() {
	// implant pprof profiler
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	// Define command-line flags
	source := flag.String("s", "", "Source directory path")
	target := flag.String("t", "", "Target directory path")
	threads := flag.Uint("mt", 0, "Number of threads to use")

	// Parse command-line arguments
	flag.Parse()

	// Check if required flags are provided
	if *source == "" || *target == "" || *threads == 0 {
		fmt.Println("Usage: -source <source_directory> -target <target_directory> -threads <number_of_threads>")
		return
	}

	// Use the provided arguments
	args := Args{
		Source:  *source,
		Target:  *target,
		Threads: *threads,
	}

	sourcePath := args.Source
	targetPath := args.Target

	// check if the source folder existed.
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		fmt.Println("Source must be a directory.")
		return
	}

	// create the target folder if it doesn't exist.
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		os.MkdirAll(targetPath, os.ModePerm)
	}

	// start timer
	start := time.Now()

	// get file and folder lists and total file count and folder count
	totalFileCount, totalSize, folders, files := getFilesAndDir(sourcePath)
	folderCount := len(folders)

	var elapsed time.Duration = time.Since(start)
	fmt.Printf("Size %s of total files / folders: %d / %d.\tElapsed time: %v\n",
		humanize.IBytes(totalSize), totalFileCount, folderCount, elapsed)

	// split it into chunks by the thread number
	folderChunkSize := uint64(folderCount) / uint64(args.Threads)
	folderChunks := chunkArray(folders, int(math.Round((float64(folderChunkSize)))))

	// Create a thread poolFolder with 4 workers
	poolFolder := NewThreadPool(int(len(folderChunks)))
	defer poolFolder.Stop()

	// Create all folders in parallel
	i := 0
	for _, folders := range folderChunks {
		folders := folders
		err := poolFolder.Submit(func() {
			createFolders(sourcePath, targetPath, folders)
		})
		i++
		if err != nil {
			// retry it after 3 seconds
			time.Sleep(1 * time.Second)
			createFolders(sourcePath, targetPath, folders)
		}
	}

	elapsed = time.Since(start)
	fmt.Printf("Created all folders in destination.\tElapsed time: %v\n", elapsed)

	// create a progress bar
	barMain := pb.StartNew(int(totalFileCount))
	barMain.SetTemplateString(`{{string . "prefix"}} {{counters . }} {{bar . "[" "=" ">" "-" "]" | green}} {{percent . }} {{etime . }} {{string . "suffix"}}`)
	barMain.Start()

	fileChunkSize := totalFileCount / uint64(args.Threads)
	fileChunks := chunkArray(files, int(math.Round((float64(fileChunkSize)))))

	// Create a thread pool for copying threads
	poolCopy := NewThreadPool(int(len(fileChunks)))

	for _, files := range fileChunks {
		files := files
		err := poolCopy.Submit(func() {
			copyFiles(sourcePath, targetPath, files, barMain)
		})

		if err != nil {
			time.Sleep(1 * time.Second)
			copyFiles(sourcePath, targetPath, files, barMain)
		}
	}

	poolCopy.Stop()
	barMain.Finish()

	elapsed = time.Since(start)
	fmt.Printf("\nTotal Elapsed time: %v\n\n", elapsed)
}

// NewThreadPool creates a new thread pool with a specified number of workers.
func NewThreadPool(numWorkers int) *ThreadPool {
	pool := &ThreadPool{
		tasks: make(chan func()),
	}
	for i := 0; i < numWorkers; i++ {
		pool.wg.Add(1)
		go func() {
			defer pool.wg.Done()
			for task := range pool.tasks {
				task()
			}
		}()
	}
	return pool
}

// Submit submits a task to the thread pool.
func (pool *ThreadPool) Submit(task func()) error {
	select {
	case pool.tasks <- task:
		return nil
	default:
		return ErrTaskQueueFull
	}
}

var ErrTaskQueueFull = errors.New("task queue is full")

// Stop stops the thread pool, waiting for all tasks to complete.
func (pool *ThreadPool) Stop() {
	close(pool.tasks)
	pool.wg.Wait()
}

func (pool *ThreadPool) Wait() {
	pool.wg.Wait()
}

func copyFiles(sourcePath, targetPath string, files []string, barMain *pb.ProgressBar) {
	for _, file := range files {
		extractedFilename := strings.Replace(file, sourcePath, "", 1)
		destFile := filepath.Join(targetPath, extractedFilename)
		// err := copyFileWithPool(file, destFile)
		err := copyFile(file, destFile)
		if err != nil {
			fmt.Printf("Error copying file %s: %v\n", file, err)
		}
		barMain.Increment()
	}
}

func copyFileWithPool(src, dst string) error {
	// Get a file handle from the file pool
	srcFile := filePool.Get().(*os.File)
	defer filePool.Put(srcFile)

	// open the source file
	var err error
	srcFile, err = os.Open(src)
	if err != nil {
		return fmt.Errorf("Cannot open source file: %w", err)
	}
	defer srcFile.Close()

	// create the target file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("Failed to create target file: %w", err)
	}
	defer dstFile.Close()

	// copy file with buffer
	_, err = io.CopyBuffer(dstFile, srcFile, make([]byte, 32*1024))
	// buf := make([]byte, 32*1024)
	// var totalBytes int64
	// for {
	// 	n, err := srcFile.Read(buf)
	// 	if err != nil && err != io.EOF {
	// 		return fmt.Errorf("Failed to read source file: %w", err)
	// 	}
	// 	if n == 0 {
	// 		break
	// 	}

	// 	if _, err := dstFile.Write(buf[:n]); err != nil {
	// 		return fmt.Errorf("Failed to write to target file: %w", err)
	// 	}

	// 	totalBytes += int64(n)
	// 	barTransfer.Add64(int64(n)) // 진행률 표시줄에 전송된 바이트 수를 업데이트합니다.
	// }
	// _, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("Failed to copy file: %w", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	// open the source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("Cannot open source file: %w", err)
	}
	defer srcFile.Close()

	// create the target file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("Failed to create target file: %w", err)
	}
	defer dstFile.Close()

	// copy file
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("Failed to copy file: %w", err)
	}

	return nil
}

func getFilesAndDir(path string) (uint64, uint64, []string, []string) {
	var filesCount, totalSize uint64
	var directories []string
	var files []string

	err := filepath.WalkDir(path, func(pathInfo string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			directories = append(directories, pathInfo)
		} else {
			filesCount++
			info, err := d.Info()
			if err != nil {
				return err
			}
			totalSize += uint64(info.Size())
			files = append(files, pathInfo)
		}

		return nil
	})
	if err != nil {
		fmt.Println("Error counting files:", err)
	}
	return filesCount, totalSize, directories, files
}

func createFolders(sourcePath string, targetPath string, folders []string) {
	for _, folder := range folders {
		relativePath, _ := filepath.Rel(sourcePath, folder)
		datFolder := filepath.Join(targetPath, relativePath)
		err := os.MkdirAll(datFolder, os.ModePerm)
		if err != nil {
			fmt.Printf("Error creating directory %s: %v\n", datFolder, err)
		}
	}
}

func chunkArray(entities []string, chunkSize int) [][]string {
	var chunks [][]string
	for i := 0; i < len(entities); i += chunkSize {
		end := i + chunkSize
		if end > len(entities) {
			end = len(entities)
		}
		chunks = append(chunks, entities[i:end])
	}
	return chunks
}

# Go Multi-Threaded Folder Copier

This is a Go console program that copies a folder using multi-threads and displays copying progress, including copied files, copied bytes, estimated time, and elapsed time.

## Usage

go run main.go -s [source_folder] -t [target_folder] -mt [thread_number]

## Example

go run main.go -s ./src -t ./copied_folder -mt 5


# Recursive File Copy Program

This program copies the files and folder structure from a source directory to a target directory using multiple threads for parallel processing.

## Usage
1. Clone the repository.
2. Compile and run the program.
3. Provide the source directory, target directory, and number of threads as command-line arguments.

go run main.go -s [source_folder] -t [target_folder] -mt [thread_number]

## Example

go run main.go -s ./src -t ./copied_folder -mt 5

## Dependencies
- `github.com/cheggaaa/pb/v3` for progress bar functionality.

## Structure
- `main.go`: Main program file.
- `README.md`: Instructions and information about the program.

## How to Run
1. Compile the program using `go build main.go`.
2. Run the program with the source directory, target directory, and number of threads as arguments.

## Author
JK NOH

## License
GPL

Feel free to contribute or report issues!
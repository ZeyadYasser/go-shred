# go-shred

This is the repository for exercise 2 of my techincal task (Shred tool in Go).

## Quickstart
Install using `go get gitlab.com/zeyad.y.g/shred`

```golang
import (
	"log"

	"gitlab.com/zeyad.y.g/shred"
)

func main() {
	err := shred.Shred("test_file")
	if err != nil {
		log.Fatalf("failed to shred file: %s", err)
	}
}
```

## Run tests
Run all tests using `go test -v`.

To generate coverage report:
- Run `go test -v -coverprofile cover.out`
- Run `go tool cover -html cover.out -o cover.html`
- Open generated `cover.html` file.

Some tests require running as root like `TestShredBlockDevice`.
This is not mandatory and will be skipped if the dependencies are not met.

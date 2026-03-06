package main

import (
	"fmt"
	"os"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/auth"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: hashpassword <password>")
	}

	hash, err := auth.HashPassword(os.Args[1])
	if err != nil {
		panic(err)
	}

	fmt.Print(hash)
}

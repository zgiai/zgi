package main

import (
	"context"
	"log"
)

func main() {
	if err := run(context.Background(), log.Default()); err != nil {
		log.Fatal(err)
	}
}

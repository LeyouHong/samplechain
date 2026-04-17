package main

import (
	"os"

	"github.com/LeyouHong/samplechain/cli"
)

func main() {
	defer os.Exit(0)
	cli := cli.CommandLine{}
	cli.Run()
}

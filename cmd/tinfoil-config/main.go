package main

import (
	"flag"
	"fmt"
	"os"

	tinfoilconfig "github.com/tinfoilsh/tinfoil-config"
)

func main() {
	hostDebug := flag.Bool("host-debug", false, "validate a config after trusted tinfoild debug injection")
	toJSON := flag.Bool("json", false, "print the bytes that represent the config: canonical JSON from "+tinfoilconfig.JSONConfigMinVersion+" on")
	schema := flag.Bool("schema", false, "print the JSON Schema of a JSON config")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [--host-debug] [--json] CONFIG\n       %s --schema\n", os.Args[0], os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if *schema {
		os.Stdout.Write(tinfoilconfig.JSONSchema())
		return
	}
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read config: %v\n", err)
		os.Exit(1)
	}
	options := tinfoilconfig.Options{}
	if *hostDebug {
		options.Mode = tinfoilconfig.HostDebugMode
	}
	if *toJSON {
		normalized, err := tinfoilconfig.Normalize(data, options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid tinfoil config: %v\n", err)
			os.Exit(1)
		}
		os.Stdout.Write(normalized)
		return
	}
	if err := tinfoilconfig.ValidateBytes(data, options); err != nil {
		fmt.Fprintf(os.Stderr, "invalid tinfoil config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("valid tinfoil config: %s\n", flag.Arg(0))
}

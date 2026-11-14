// Command streameraid draws Mermaid diagrams as SVG.
//
//	streameraid render [flags] [in.mmd]
//
//	-o file          write the SVG to file instead of the standard output
//	-dark            draw for a dark background
//	-bg color        fill the background (transparent by default)
//	-font-size n     the size of labels (14 by default)
//
// It reads the standard input when no file is given, or the file is "-".
// Flags may come before or after the file. Statements it cannot read are
// reported on the standard error, and the diagram is drawn without them;
// the exit status is then 0 still, 1 when nothing could be drawn, and 2
// for a wrong command line.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/momaek/streameraid"
)

const usage = `usage: streameraid render [flags] [in.mmd]

Draws a Mermaid flowchart or sequence diagram as SVG, reading the standard
input when no file is given.

  -o file          write the SVG to file instead of the standard output
  -dark            draw for a dark background
  -bg color        fill the background (transparent by default)
  -font-size n     the size of labels (14 by default)
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run runs the command and returns its exit status.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stderr, usage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	if args[0] != "render" {
		fmt.Fprintf(stderr, "streameraid: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	out := fs.String("o", "", "")
	dark := fs.Bool("dark", false, "")
	bg := fs.String("bg", "", "")
	size := fs.Float64("font-size", 14, "")
	// Flags may come after the file as well: parse, take a file, parse on.
	var files []string
	rest := args[1:]
	for {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		files = append(files, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(files) > 1 {
		fmt.Fprintln(stderr, "streameraid: one input file at most")
		return 2
	}
	var src []byte
	var err error
	if len(files) == 0 || files[0] == "-" {
		src, err = io.ReadAll(stdin)
	} else {
		src, err = os.ReadFile(files[0])
	}
	if err != nil {
		fmt.Fprintln(stderr, "streameraid:", err)
		return 1
	}
	res, err := streameraid.Render(string(src), streameraid.Options{Dark: *dark, Background: *bg, FontSize: *size})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, p := range res.Problems {
		if p.Line > 0 {
			fmt.Fprintf(stderr, "line %d: %s: %s\n", p.Line, p.Msg, p.Text)
		} else {
			fmt.Fprintln(stderr, p.Msg)
		}
	}
	if *out == "" {
		io.WriteString(stdout, res.SVG)
		return 0
	}
	if err := os.WriteFile(*out, []byte(res.SVG), 0o644); err != nil {
		fmt.Fprintln(stderr, "streameraid:", err)
		return 1
	}
	return 0
}

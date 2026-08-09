package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ---- RESP serializers -------------------------------------------------

const nullBulk = "$-1\r\n"

func simpleString(s string) string { return "+" + s + "\r\n" }
func errorReply(m string) string   { return "-" + m + "\r\n" }
func integer(n int) string         { return fmt.Sprintf(":%d\r\n", n) }

func bulkString(s string) string {
	return fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
}

// ---- Handlers ---------------------------------------------------------

// handlerFunc receives ONLY the arguments. args[0] is the first argument,
// not the command name. The dispatcher strips and normalizes the name.
type handlerFunc func(args []string) string

type command struct {
	fn       handlerFunc
	min, max int
}


func cmdPing(args []string) string {
	if len(args) > 0 {
		return bulkString(args[0])
	}
	return simpleString("PONG")
}

func cmdEcho(args []string) string {
	return bulkString(args[0])
}

func cmdCommand(args []string) string {
	return simpleString("OK")
}



var commands = map[string]command{
	"PING": {cmdPing, 0, 1},
	"ECHO": {cmdEcho, 1, 1},
	"COMMAND": {cmdCommand, 0, 1},
}

// ---- Dispatch ---------------------------------------------------------

func handle(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	cmd, args := strings.ToUpper(parts[0]), parts[1:]

	h, ok := commands[cmd]
	if !ok {
		return errorReply(fmt.Sprintf("ERR unknown command '%s'", parts[0]))
	}
	if len(args) < h.min || len(args) > h.max {
		return errorReply(fmt.Sprintf("ERR wrong number of arguments for '%s' command", cmd))
	}
	return h.fn(args)
}

// ---- Line parsing (temporary; replaced by RESP arrays later) ----------

func parseArgs(line string) []string {
	var args []string
	var cur strings.Builder
	inQ := false
	for _, ch := range line {
		switch {
		case ch == '"' && !inQ:
			inQ = true
		case ch == '"' && inQ:
			inQ = false
		case ch == ' ' && !inQ:
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fmt.Print(handle(parseArgs(line)))
	}
}
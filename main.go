package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)


// store is a simple in-memory key-value store.
var store = map[string]string{}

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

func cmdSet(args []string) string {
	key, value := args[0], args[1]
	store[key] = value
	return simpleString("OK")
}

func cmdGet(args []string) string {
	key := args[0]
	value, ok := store[key]
	if !ok {
		return nullBulk
	}
	return bulkString(value)
}

// ---- Command dispatch table -------------------------------------------

var commands = map[string]command{
	"PING": {cmdPing, 0, 1},
	"ECHO": {cmdEcho, 1, 1},
	"SET": {cmdSet, 2, 2},
	"GET": {cmdGet, 1, 1},
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

// parseRequest reads one RESP array from r and returns its arg list.
func parseRequest(r *bufio.Reader) ([]string, error) {
	// Read the *N\r\n line
	line, err := r.ReadString('\n')
	
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("expected '*', got %q", line)
	}

	arraySize, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, fmt.Errorf("invalid array size: %v", err)
	}
	
	args := make([]string, 0, arraySize)
	
	for range arraySize {
		// Read the $len\r\n line
		lenLine, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(lenLine, "$") {
			return nil, fmt.Errorf("expected '$', got %q", lenLine)
		}
		
		strLen, err := strconv.Atoi(strings.TrimSpace(lenLine[1:]))
		if err != nil {
			return nil, fmt.Errorf("invalid string length: %v", err)
		}
		
		buf := make([]byte, strLen+2) // +2 for \r\n
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		
		args = append(args, string(buf[:strLen])) // Exclude the trailing \r\n
	}

	_ = strconv.Atoi
	_ = io.ReadFull
	return args, nil	

	// TODO: 1. Parse N from line[1:] (trim \r\n)
	// TODO: 2. Loop N times: read $len\r\n, then read exactly len bytes + trailing \r\n
	// TODO: 3. Use io.ReadFull(r, buf) — DO NOT use ReadString once you know the byte length,
	//          because bulk bodies may contain \r\n.
	// TODO: 4. Return the assembled []string

	
	
}

// ---- Main loop ---------------------------------------------------------
func main() {
	r := bufio.NewReader(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for {
		args, err := parseRequest(r)
		if err != nil {
			return
		}
		w.WriteString(handle(args))
		w.Flush()
	}
}
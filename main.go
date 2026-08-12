package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var store = map[string]string{}
var expiry = map[string]int64{} // key -> absolute expiry in ms (-1 = no expiry)
var clock int64 = 0             // simulated clock in milliseconds

func isExpired(key string) bool {
	exp, ok := expiry[key]
	return ok && exp >= 0 && clock >= exp
}

func eb(s string, ok bool) string {
	if !ok { return "$-1\r\n" }
	return fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
}
func es(s string) string { return fmt.Sprintf("+%s\r\n", s) }
func ee(m string) string { return fmt.Sprintf("-%s\r\n", m) }
func ei(n int) string    { return fmt.Sprintf(":%d\r\n", n) }

func incrBy(key string, delta int) string {
	v, ok := store[key]
	if !ok { v = "0" }
	n, err := strconv.Atoi(v)
	if err != nil { return ee("ERR value is not an integer or out of range") }
	n += delta
	store[key] = strconv.Itoa(n)
	return ei(n)
}

func setCmd(args []string) string {
	if len(args) < 3 { return ee("ERR wrong number of arguments for 'SET' command") }
	key, val := args[1], args[2]
	nx, xx := false, false
	for i := 3; i < len(args); i++ {
		switch strings.ToUpper(args[i]) {
		case "NX": nx = true
		case "XX": xx = true
		}
	}
	_, exists := store[key]
	if nx && exists { return eb("", false) }
	if xx && !exists { return eb("", false) }
	store[key] = val
	expiry[key] = -1 // no expiry by default
	return es("OK")
}

func handle(args []string) string {
	cmd := strings.ToUpper(args[0])
	switch cmd {
	case "PING":
		if len(args) > 2 { return ee("ERR wrong number of arguments for 'PING' command") }
		if len(args) == 1 { return es("PONG") }
		return eb(args[1], true)
	case "ECHO":
		if len(args) != 2 { return ee("ERR wrong number of arguments for 'ECHO' command") }
		return eb(args[1], true)
	case "COMMAND":
		return es("OK")
	case "SET":
		return setCmd(args)
	case "GET":
		v, ok := store[args[1]]
		return eb(v, ok)
	case "DBSIZE":
		return ei(len(store))
	case "INCR":
		return incrBy(args[1], 1)
	case "DECR":
		return incrBy(args[1], -1)
	case "INCRBY":
		amt, err := strconv.Atoi(args[2])
		if err != nil { return ee("ERR value is not an integer or out of range") }
		return incrBy(args[1], amt)
	case "DECRBY":
		amt, err := strconv.Atoi(args[2])
		if err != nil { return ee("ERR value is not an integer or out of range") }
		return incrBy(args[1], -amt)
	case "EXPIRE":
		exp, err := strconv.Atoi(args[2])
		if err != nil { return ee("ERR value is not an integer or out of range") }
		if _, exists := store[args[1]]; !exists { return ei(0) }
		expiry[args[1]] = clock + int64(exp*1000)
		return ei(1)
	case "TTL":
		if _, exists := store[args[1]]; !exists { return ei(-2) }
		if expiry[args[1]] == -1 { return ei(-1) }
		return ei(int((expiry[args[1]] - clock) / 1000))
	case "PERSIST":
		exp, exists := expiry[args[1]]
    	if !exists || exp == -1 { return ei(0) }
    	expiry[args[1]] = -1
   		return ei(1)
	case "WAIT":
		// Advance simulated clock by args[1] milliseconds
		ms, _ := strconv.ParseInt(args[1], 10, 64)
		clock += ms
		return es("OK")
	}
	return ee(fmt.Sprintf("ERR unknown command '%s'", args[0]))
}

func parseArgs(line string) []string {
	var args []string
	var cur strings.Builder
	inQ := false
	for _, ch := range line {
		switch {
		case ch == '"' && !inQ: inQ = true
		case ch == '"' && inQ: inQ = false
		case ch == ' ' && !inQ:
			if cur.Len() > 0 { args = append(args, cur.String()); cur.Reset() }
		default: cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 { args = append(args, cur.String()) }
	return args
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" { continue }
		fmt.Print(handle(parseArgs(line)))
	}
}

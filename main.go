package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var store = map[string]string{}
var expiry = map[string]int64{}
var lists = map[string][]string{}
var keyType = map[string]string{} // "string" or "list"
var clock int64 = 0

func isExpired(key string) bool {
	exp, ok := expiry[key]
	return ok && exp >= 0 && clock >= exp
}

func checkExpiry(key string) {
	if isExpired(key) {
		delete(store, key); delete(expiry, key)
		delete(lists, key); delete(keyType, key)
	}
}

func wrongType(key, want string) string {
	if t, ok := keyType[key]; ok && t != want {
		return ee("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	return ""
}

func eb(s string, ok bool) string {
	if !ok { return "$-1\r\n" }
	return fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
}
func es(s string) string { return fmt.Sprintf("+%s\r\n", s) }
func ee(m string) string { return fmt.Sprintf("-%s\r\n", m) }
func ei(n int) string    { return fmt.Sprintf(":%d\r\n", n) }
func ea(items []string) string {
	r := fmt.Sprintf("*%d\r\n", len(items))
	for _, it := range items { r += eb(it, true) }
	return r
}

func incrBy(key string, delta int) string {
	checkExpiry(key)
	if e := wrongType(key, "string"); e != "" { return e }
	v, ok := store[key]
	if !ok { v = "0" }
	n, err := strconv.Atoi(v)
	if err != nil { return ee("ERR value is not an integer or out of range") }
	n += delta
	store[key] = strconv.Itoa(n)
	keyType[key] = "string"
	return ei(n)
}

func setCmd(args []string) string {
	if len(args) < 3 { return ee("ERR wrong number of arguments for 'SET' command") }
	key, val := args[1], args[2]
	nx, xx := false, false
	exMs := int64(-1)
	for i := 3; i < len(args); i++ {
		switch strings.ToUpper(args[i]) {
		case "NX": nx = true
		case "XX": xx = true
		case "EX":
			i++; secs, _ := strconv.ParseInt(args[i], 10, 64); exMs = secs * 1000
		case "PX":
			i++; ms, _ := strconv.ParseInt(args[i], 10, 64); exMs = ms
		}
	}
	_, exists := store[key]
	if !exists { _, exists = keyType[key] } // check if any type exists
	if nx && exists { return eb("", false) }
	if xx && !exists { return eb("", false) }
	// SET always overwrites regardless of previous type
	delete(lists, key)
	store[key] = val
	keyType[key] = "string"
	if exMs >= 0 { expiry[key] = clock + exMs } else { expiry[key] = -1 }
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
		checkExpiry(args[1])
		if e := wrongType(args[1], "string"); e != "" { return e }
		v, ok := store[args[1]]
		return eb(v, ok)
	case "EXISTS":
		cnt := 0
        for _, k := range args[1:] {
        checkExpiry(k)
        if _, ok := keyType[k]; ok { cnt++ }
        }
    	return ei(cnt)	
	case "DBSIZE":
		cnt := 0
		for k := range keyType { checkExpiry(k); if _, ok := keyType[k]; ok { cnt++ } }
		return ei(cnt)
	case "INCR": return incrBy(args[1], 1)
	case "DECR": return incrBy(args[1], -1)
	case "INCRBY":
		amt, err := strconv.Atoi(args[2])
		if err != nil { return ee("ERR value is not an integer or out of range") }
		return incrBy(args[1], amt)
	case "DECRBY":
		amt, err := strconv.Atoi(args[2])
		if err != nil { return ee("ERR value is not an integer or out of range") }
		return incrBy(args[1], -amt)
	case "EXPIRE":
		checkExpiry(args[1])
		if _, ok := keyType[args[1]]; !ok { return ei(0) }
		secs, _ := strconv.ParseInt(args[2], 10, 64)
		expiry[args[1]] = clock + secs*1000
		return ei(1)
	case "TTL":
		checkExpiry(args[1])
		if _, ok := keyType[args[1]]; !ok { return ei(-2) }
		exp := expiry[args[1]]
		if exp < 0 { return ei(-1) }
		return ei(int((exp - clock) / 1000))
	case "PTTL":
		checkExpiry(args[1])
		if _, ok := keyType[args[1]]; !ok { return ei(-2) }
		exp := expiry[args[1]]
		if exp < 0 { return ei(-1) }
		return ei(int(exp - clock))
	case "PERSIST":
		checkExpiry(args[1])
		if _, ok := keyType[args[1]]; !ok { return ei(0) }
		exp := expiry[args[1]]
		if exp < 0 { return ei(0) }
		expiry[args[1]] = -1
		return ei(1)
	case "WAIT":
		ms, _ := strconv.ParseInt(args[1], 10, 64)
		clock += ms
		return es("OK")
	case "LPUSH":
		checkExpiry(args[1])
		if e := wrongType(args[1], "list"); e != "" { return e }
		key := args[1]		
		for _, val := range args[2:] {
			if _, ok := lists[key]; !ok { lists[key] = []string{} }
			lists[key] = append([]string{val}, lists[key]...)
		}
		keyType[key] = "list"
		return ei(len(lists[key]))
	case "RPUSH":
		checkExpiry(args[1])
		if e := wrongType(args[1], "list"); e != "" { return e }
		key := args[1]
		if _, ok := lists[key]; !ok { lists[key] = []string{} }	
		// TODO: Append each value in args[2:] to lists[key]
		// Create list if doesn't exist. Set keyType[key] = "list"
		// Return ei(len(lists[key]))

		for _, val := range args[2:] {
			lists[key] = append(lists[key], val)
		}
		keyType[key] = "list"
		return ei(len(lists[key]))	
	case "LRANGE":
		checkExpiry(args[1])
		key := args[1]
		if _, ok := lists[key]; !ok { return ea(nil) }
		start, _ := strconv.Atoi(args[2])
		stop, _ := strconv.Atoi(args[3])
		l := lists[key]; ln := len(l)
		if start < 0 { start = ln + start }
		if stop < 0 { stop = ln + stop }
		if start < 0 { start = 0 }
		if stop >= ln { stop = ln - 1 }
		if start > stop { return ea(nil) }
		return ea(l[start : stop+1])
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

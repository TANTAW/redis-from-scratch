package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

var store = map[string]string{}
var expiry = map[string]int64{}
var lists = map[string][]string{}
var hashes = map[string]map[string]string{}
var sets = map[string]map[string]bool{}
var keyType = map[string]string{}
var clock int64 = 0

func isExpired(key string) bool {
	exp, ok := expiry[key]
	return ok && exp >= 0 && clock >= exp
}
func checkExpiry(key string) {
	if isExpired(key) { delete(store, key); delete(expiry, key); delete(lists, key); delete(hashes, key); delete(keyType, key) }
}
func wrongType(key, want string) string {
	if t, ok := keyType[key]; ok && t != want { return ee("WRONGTYPE Operation against a key holding the wrong kind of value") }
	return ""
}

func cleanupEmpty(key string) {
	if l, ok := lists[key]; ok && len(l) == 0 { delete(lists, key); delete(keyType, key); delete(expiry, key) }
	if h, ok := hashes[key]; ok && len(h) == 0 { delete(hashes, key); delete(keyType, key); delete(expiry, key) }
}

func delKey(key string) {
	delete(store, key); delete(expiry, key); delete(lists, key)
	delete(hashes, key); delete(sets, key); delete(keyType, key)
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
		case "EX": i++; s, _ := strconv.ParseInt(args[i], 10, 64); exMs = s * 1000
		case "PX": i++; ms, _ := strconv.ParseInt(args[i], 10, 64); exMs = ms
		}
	}
	_, exists := keyType[key]
	if nx && exists { return eb("", false) }
	if xx && !exists { return eb("", false) }
	delete(lists, key)
	store[key] = val; keyType[key] = "string"
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
	case "COMMAND": return es("OK")
	case "SET": return setCmd(args)
	case "GET":
		checkExpiry(args[1])
		if e := wrongType(args[1], "string"); e != "" { return e }
		v, ok := store[args[1]]; return eb(v, ok)
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
		expiry[args[1]] = clock + secs*1000; return ei(1)
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
		expiry[args[1]] = -1; return ei(1)
	case "WAIT":
		ms, _ := strconv.ParseInt(args[1], 10, 64); clock += ms; return es("OK")
	case "LPUSH":
		checkExpiry(args[1])
		if e := wrongType(args[1], "list"); e != "" { return e }
		key := args[1]
		if _, ok := lists[key]; !ok { lists[key] = nil; keyType[key] = "list" }
		for _, v := range args[2:] { lists[key] = append([]string{v}, lists[key]...) }
		return ei(len(lists[key]))
	case "RPUSH":
		checkExpiry(args[1])
		if e := wrongType(args[1], "list"); e != "" { return e }
		key := args[1]
		if _, ok := lists[key]; !ok { lists[key] = nil; keyType[key] = "list" }
		lists[key] = append(lists[key], args[2:]...)
		return ei(len(lists[key]))
	case "LPOP":
		checkExpiry(args[1])
		key := args[1]
		l, ok := lists[key]
		if !ok || len(l) == 0 { return eb("", false) }
		val := l[0]
		lists[key] = l[1:]
		if len(lists[key]) == 0 { cleanupEmpty(key) }
		return eb(val, true)

	case "RPOP":
		checkExpiry(args[1])
		key := args[1]
		l, ok := lists[key]
		if !ok || len(l) == 0 { return eb("", false) }
		val := l[len(l)-1]
		lists[key] = l[:len(l)-1]
		if len(lists[key]) == 0 { cleanupEmpty(key) }
		return eb(val, true)		
	
	case "LLEN":
		checkExpiry(args[1])
		key := args[1]
		if _, ok := lists[key]; !ok { return ei(0) }
		return ei(len(lists[key]))
	case "LRANGE":
		checkExpiry(args[1])
		key := args[1]
		if _, ok := lists[key]; !ok { return ea(nil) }
		start, _ := strconv.Atoi(args[2]); stop, _ := strconv.Atoi(args[3])
		l := lists[key]; ln := len(l)
		if start < 0 { start = ln + start }; if stop < 0 { stop = ln + stop }
		if start < 0 { start = 0 }; if stop >= ln { stop = ln - 1 }
		if start > stop { return ea(nil) }
		return ea(l[start : stop+1])
	case "HSET":
		checkExpiry(args[1]); if e := wrongType(args[1], "hash"); e != "" { return e }
		key := args[1]
		h, ok := hashes[key]
		if !ok { h = map[string]string{}; hashes[key] = h; keyType[key] = "hash" }
		newCount := 0
		for i := 2; i < len(args); i += 2 {
			field, value := args[i], args[i+1]
			if _, exists := h[field]; !exists { newCount++ }
			h[field] = value
		}
		return ei(newCount)
	case "HGET":
		checkExpiry(args[1]); if e := wrongType(args[1], "hash"); e != "" { return e }
		var key = args[1]	
		var field = args[2]
		if h, ok := hashes[key]; ok {
			if val, exists := h[field]; exists {
				return eb(val, true)
			}
		}
		return eb("", false)
	
	case "HDEL":
		checkExpiry(args[1]); if e := wrongType(args[1], "hash"); e != "" { return e }
		hash, ok := hashes[args[1]]
		if !ok { return ei(0) }
		count := 0
		for _, field := range args[2:] {
			if _, exists := hash[field]; exists {
				delete(hash, field)
				count++
			}
		}
		cleanupEmpty(args[1])
		return ei(count)
	case "HGETALL":
		checkExpiry(args[1]); if e := wrongType(args[1], "hash"); e != "" { return e }
		list, ok := hashes[args[1]]
		if !ok { return ea(nil) }
		result := make([]string, 0, len(list)*2)
		for field, value := range list {
			result = append(result, field, value)
		}
		return ea(result)
	case "HEXISTS":
		checkExpiry(args[1])
		hash, ok := hashes[args[1]]
		if !ok { return ei(0) }
		if _, exists := hash[args[2]]; exists {
			return ei(1)
		}
		return ei(0)
	case "HLEN":
		checkExpiry(args[1])
		if e := wrongType(args[1], "hash"); e != "" { return e }
		hash, ok := hashes[args[1]]
		if !ok { return ei(0) }
		return ei(len(hash))
	
	case "SADD":
		checkExpiry(args[1]); if e := wrongType(args[1], "set"); e != "" { return e }
		key := args[1]
		set, ok := sets[key]
		if !ok { set = map[string]bool{}; sets[key] = set; keyType[key] = "set" }
		newCount := 0
		for _, member := range args[2:] {
			if !set[member] {
				set[member] = true
				newCount++
			}
		}
		return ei(newCount)
	case "SMEMBERS":
		checkExpiry(args[1]); if e := wrongType(args[1], "set"); e != "" { return e }
		set, ok := sets[args[1]]
		if !ok { return ea(nil) }
		result := make([]string, 0, len(set))
		for member := range set {
			result = append(result, member)
		}
		sort.Strings(result)
		return ea(result)
	case "SISMEMBER":
		checkExpiry(args[1])
		set, ok := sets[args[1]]
		if !ok { return ei(0) }
		if _, exists := set[args[2]]; exists {
			return ei(1)
		}
		return ei(0)
	case "SCARD":
		checkExpiry(args[1])
		set, ok := sets[args[1]]
		if !ok { return ei(0) }
		return ei(len(set))
	case "SREM":
		checkExpiry(args[1]); if e := wrongType(args[1], "set"); e != "" { return e }
		set, ok := sets[args[1]]
		if !ok { return ei(0) }
		count := 0
		for _, member := range args[2:] {
			if _, exists := set[member]; exists {
				delete(set, member)
				count++
			}
		}
		cleanupEmpty(args[1])
		return ei(count)

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

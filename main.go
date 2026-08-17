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

type zEntry struct { member string; score float64 }
var zsets = map[string][]zEntry{}

var keyType = map[string]string{}
var clock int64 = 0

func isExpired(key string) bool { exp, ok := expiry[key]; return ok && exp >= 0 && clock >= exp }
func checkExpiry(key string) {
	if isExpired(key) { delete(store, key); delete(expiry, key); delete(lists, key); delete(hashes, key); delete(sets, key); delete(zsets, key); delete(keyType, key) }
}
func wrongType(key, want string) string {
	if t, ok := keyType[key]; ok && t != want { return ee("WRONGTYPE Operation against a key holding the wrong kind of value") }
	return ""
}
func delKey(key string) {
	delete(store, key); delete(expiry, key); delete(lists, key)
	delete(hashes, key); delete(sets, key); delete(zsets, key); delete(keyType, key)
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
	checkExpiry(key); if e := wrongType(key, "string"); e != "" { return e }
	v, ok := store[key]; if !ok { v = "0" }
	n, err := strconv.Atoi(v)
	if err != nil { return ee("ERR value is not an integer or out of range") }
	n += delta; store[key] = strconv.Itoa(n); keyType[key] = "string"; return ei(n)
}

func setCmd(args []string) string {
	if len(args) < 3 { return ee("ERR wrong number of arguments for 'SET' command") }
	key, val := args[1], args[2]
	nx, xx := false, false; exMs := int64(-1)
	for i := 3; i < len(args); i++ {
		switch strings.ToUpper(args[i]) {
		case "NX": nx = true; case "XX": xx = true
		case "EX": i++; s, _ := strconv.ParseInt(args[i], 10, 64); exMs = s * 1000
		case "PX": i++; ms, _ := strconv.ParseInt(args[i], 10, 64); exMs = ms
		}
	}
	_, exists := keyType[key]
	if nx && exists { return eb("", false) }
	if xx && !exists { return eb("", false) }
	delete(lists, key); delete(hashes, key); delete(sets, key); delete(zsets, key)
	store[key] = val; keyType[key] = "string"
	if exMs >= 0 { expiry[key] = clock + exMs } else { expiry[key] = -1 }
	return es("OK")
}

func cleanupEmpty(key string) {
	if l, ok := lists[key]; ok && len(l) == 0 { delKey(key) }
	if h, ok := hashes[key]; ok && len(h) == 0 { delKey(key) }
	if s, ok := sets[key]; ok && len(s) == 0 { delKey(key) }
	if z, ok := zsets[key]; ok && len(z) == 0 { delKey(key) }
}

func zsortedEntries(key string) []zEntry {
	entries := make([]zEntry, len(zsets[key]))
	copy(entries, zsets[key])
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].score != entries[j].score { return entries[i].score < entries[j].score }
		return entries[i].member < entries[j].member
	})
	return entries
}

func handle(args []string) string {
	cmd := strings.ToUpper(args[0])
	switch cmd {
	case "PING":
		if len(args) > 2 { return ee("ERR wrong number of arguments for 'PING' command") }
		if len(args) == 1 { return es("PONG") }; return eb(args[1], true)
	case "ECHO":
		if len(args) != 2 { return ee("ERR wrong number of arguments for 'ECHO' command") }
		return eb(args[1], true)
	case "COMMAND": return es("OK")
	case "SET": return setCmd(args)
	case "GET":
		checkExpiry(args[1]); if e := wrongType(args[1], "string"); e != "" { return e }
		v, ok := store[args[1]]; return eb(v, ok)
	case "DBSIZE":
		cnt := 0
		for k := range keyType { checkExpiry(k); if _, ok := keyType[k]; ok { cnt++ } }
		return ei(cnt)
	case "INCR": return incrBy(args[1], 1)
	case "DECR": return incrBy(args[1], -1)
	case "INCRBY":
		amt, err := strconv.Atoi(args[2])
		if err != nil { return ee("ERR value is not an integer or out of range") }; return incrBy(args[1], amt)
	case "DECRBY":
		amt, err := strconv.Atoi(args[2])
		if err != nil { return ee("ERR value is not an integer or out of range") }; return incrBy(args[1], -amt)
	case "EXPIRE":
		checkExpiry(args[1]); if _, ok := keyType[args[1]]; !ok { return ei(0) }
		secs, _ := strconv.ParseInt(args[2], 10, 64); expiry[args[1]] = clock + secs*1000; return ei(1)
	case "TTL":
		checkExpiry(args[1]); if _, ok := keyType[args[1]]; !ok { return ei(-2) }
		exp := expiry[args[1]]; if exp < 0 { return ei(-1) }; return ei(int((exp - clock) / 1000))
	case "PTTL":
		checkExpiry(args[1]); if _, ok := keyType[args[1]]; !ok { return ei(-2) }
		exp := expiry[args[1]]; if exp < 0 { return ei(-1) }; return ei(int(exp - clock))
	case "PERSIST":
		checkExpiry(args[1]); if _, ok := keyType[args[1]]; !ok { return ei(0) }
		if expiry[args[1]] < 0 { return ei(0) }; expiry[args[1]] = -1; return ei(1)
	case "WAIT":
		ms, _ := strconv.ParseInt(args[1], 10, 64); clock += ms; return es("OK")
	case "LPUSH":
		checkExpiry(args[1]); if e := wrongType(args[1], "list"); e != "" { return e }
		key := args[1]
		if _, ok := lists[key]; !ok { lists[key] = nil; keyType[key] = "list" }
		for _, v := range args[2:] { lists[key] = append([]string{v}, lists[key]...) }
		return ei(len(lists[key]))
	case "RPUSH":
		checkExpiry(args[1]); if e := wrongType(args[1], "list"); e != "" { return e }
		key := args[1]
		if _, ok := lists[key]; !ok { lists[key] = nil; keyType[key] = "list" }
		lists[key] = append(lists[key], args[2:]...); return ei(len(lists[key]))
	case "LPOP":
		checkExpiry(args[1]); l := lists[args[1]]; if len(l) == 0 { return eb("", false) }
		val := l[0]; lists[args[1]] = l[1:]; cleanupEmpty(args[1]); return eb(val, true)
	case "RPOP":
		checkExpiry(args[1]); l := lists[args[1]]; if len(l) == 0 { return eb("", false) }
		val := l[len(l)-1]; lists[args[1]] = l[:len(l)-1]; cleanupEmpty(args[1]); return eb(val, true)
	case "LLEN":
		checkExpiry(args[1]); if _, ok := lists[args[1]]; !ok { return ei(0) }
		return ei(len(lists[args[1]]))
	case "LRANGE":
		checkExpiry(args[1]); key := args[1]
		if _, ok := lists[key]; !ok { return ea(nil) }
		start, _ := strconv.Atoi(args[2]); stop, _ := strconv.Atoi(args[3])
		l := lists[key]; ln := len(l)
		if start < 0 { start = ln + start }; if stop < 0 { stop = ln + stop }
		if start < 0 { start = 0 }; if stop >= ln { stop = ln - 1 }
		if start > stop { return ea(nil) }; return ea(l[start : stop+1])
	case "HSET":
		checkExpiry(args[1]); if e := wrongType(args[1], "hash"); e != "" { return e }
		key := args[1]
		if _, ok := hashes[key]; !ok { hashes[key] = map[string]string{}; keyType[key] = "hash" }
		nc := 0
		for i := 2; i+1 < len(args); i += 2 { if _, ok := hashes[key][args[i]]; !ok { nc++ }; hashes[key][args[i]] = args[i+1] }
		return ei(nc)
	case "HGET":
		checkExpiry(args[1]); if e := wrongType(args[1], "hash"); e != "" { return e }
		h, ok := hashes[args[1]]; if !ok { return eb("", false) }
		v, ok := h[args[2]]; return eb(v, ok)
	case "HDEL":
		checkExpiry(args[1]); if e := wrongType(args[1], "hash"); e != "" { return e }
		h, ok := hashes[args[1]]; if !ok { return ei(0) }
		cnt := 0
		for _, f := range args[2:] { if _, ok := h[f]; ok { delete(h, f); cnt++ } }
		cleanupEmpty(args[1]); return ei(cnt)
	case "HGETALL":
		checkExpiry(args[1]); if e := wrongType(args[1], "hash"); e != "" { return e }
		h, ok := hashes[args[1]]; if !ok { return ea(nil) }
		ks := make([]string, 0, len(h))
		for k := range h { ks = append(ks, k) }; sort.Strings(ks)
		flat := make([]string, 0, len(h)*2)
		for _, k := range ks { flat = append(flat, k, h[k]) }; return ea(flat)
	case "HEXISTS":
		checkExpiry(args[1]); h, ok := hashes[args[1]]; if !ok { return ei(0) }
		if _, ok := h[args[2]]; ok { return ei(1) }; return ei(0)
	case "HLEN":
		checkExpiry(args[1]); h, ok := hashes[args[1]]; if !ok { return ei(0) }; return ei(len(h))
	case "SADD":
		checkExpiry(args[1]); if e := wrongType(args[1], "set"); e != "" { return e }
		key := args[1]
		if _, ok := sets[key]; !ok { sets[key] = map[string]bool{}; keyType[key] = "set" }
		nc := 0
		for _, m := range args[2:] { if !sets[key][m] { nc++; sets[key][m] = true } }
		return ei(nc)
	case "SMEMBERS":
		checkExpiry(args[1]); s, ok := sets[args[1]]; if !ok { return ea(nil) }
		ms := make([]string, 0, len(s))
		for m := range s { ms = append(ms, m) }; sort.Strings(ms); return ea(ms)
	case "SISMEMBER":
		checkExpiry(args[1]); s, ok := sets[args[1]]; if !ok { return ei(0) }
		if s[args[2]] { return ei(1) }; return ei(0)
	case "SCARD":
		checkExpiry(args[1]); s, ok := sets[args[1]]; if !ok { return ei(0) }; return ei(len(s))
	case "SREM":
		checkExpiry(args[1]); if e := wrongType(args[1], "set"); e != "" { return e }
		s, ok := sets[args[1]]; if !ok { return ei(0) }
		cnt := 0
		for _, m := range args[2:] { if s[m] { delete(s, m); cnt++ } }
		cleanupEmpty(args[1]); return ei(cnt)
	case "ZADD":
		checkExpiry(args[1]); if e := wrongType(args[1], "zset"); e != "" { return e }
		key := args[1]; if _, ok := zsets[key]; !ok { zsets[key] = nil; keyType[key] = "zset" }
		newCount := 0
		for i := 2; i+1 < len(args); i += 2 {
			score, err := strconv.ParseFloat(args[i], 64)
			if err != nil { return ee("ERR value is not a valid float") }
			member := args[i+1]
			found := false
			for j, entry := range zsets[key] {
				if entry.member == member {
					zsets[key][j].score = score
					found = true
					break
				}
			}
			if !found {
				zsets[key] = append(zsets[key], zEntry{member: member, score: score})
				newCount++
			}
		}
		return ei(newCount)
		
	case "ZSCORE":
		checkExpiry(args[1])
		for _, entry := range zsets[args[1]] {
			if entry.member == args[2] {
				return eb(strconv.FormatFloat(entry.score, 'g', -1, 64), true)
			}
		}
		return eb("", false)
	case "ZRANGE":
		checkExpiry(args[1])
		val := args[1]
		sortedEntries := zsortedEntries(val)
		start, _ := strconv.Atoi(args[2])
		stop, _ := strconv.Atoi(args[3])
		if start < 0 { start = len(sortedEntries) + start }
		if stop < 0 { stop = len(sortedEntries) + stop }
		if start < 0 { start = 0 }
		if stop >= len(sortedEntries) { stop = len(sortedEntries) - 1 }
		if start > stop { return ea(nil) }
		result := []string{}
		for i := start; i <= stop; i++ {
			result = append(result, sortedEntries[i].member)
			if len(args) > 4 && strings.ToUpper(args[4]) == "WITHSCORES" {
				result = append(result, strconv.FormatFloat(sortedEntries[i].score, 'g', -1, 64))
			}
		}
		return ea(result)	
	case "ZRANK":
		checkExpiry(args[1])
		val := args[2]
		sortedEntries := zsortedEntries(args[1])
		for i, entry := range sortedEntries {
			if entry.member == val {
				return ei(i) 
			}
		}
		return ei(-1) 
	case "ZCARD":
		checkExpiry(args[1])
		val := args[1]
		sortedEntries := zsortedEntries(val)
		return ei(len(sortedEntries))		

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

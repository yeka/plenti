package common

import (
	"fmt"
	"hash/crc32"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FData todo: maybe keep local and have fucntions to do the work
type FData struct {
	// store content/css/ssr per component/layout.
	// B can be the concatenated css for bundle.css, the layout file bytes or the compiled component.
	// If memory is an issue maybe let a flag decide what we keep in mem.
	B []byte
	// A component can have a script and maybe style so this applies in the latter case else it is nil.
	// Might be a litle confusing to see stylePath storing/using B and CSS getting appended but CSS is each component's css
	// of which we combine all.
	CSS []byte
	// Used to store prev compiled ssr which is reused if no changes to layout file.
	SSR []byte
	// flag that is used for now to signify that once file has been processed once it doesn't need to be done again.
	// This is specific to gopack for now anyway
	// Hash of last check
	Hash      uint32
	Processed bool
}

func (f *FData) String() string {

	return fmt.Sprintf("Hash -> %d\nContent -> %s\n", f.Hash, f.B)
}

// UseMemFS determines if local dev files are stored on disk or in memory
var UseMemFS = false
var crc32q = crc32.MakeTable(0xD5828281)

// CRC32Hasher is a simple means to check for changes
func CRC32Hasher(b []byte) uint32 {
	return crc32.Checksum(b, crc32q)

}

type MapFS struct {
	// mostly for watcher firing multiple times but also when we do concurrent work...
	mu sync.RWMutex
	// build folder files
	fs map[string]*FData
	// sorted list of files. O(Log n) look ups to start searching so hopefully better overall,
	// maybe simple map iteration is better.. will need to test on larger sized projects
	entries *[]string
	// source -> built mapping to remove any deleted files. layouts/components/foo.svelte -> public/spa/components/foo.js...
	// bad things will happen if fs and entries are not kept in sync...
	sourceToDest map[string]string
}

// should really be on a type that gets initialised and used in the build globally
var mapFS = MapFS{
	fs:      map[string]*FData{},
	entries: &[]string{},
	// might be best to keep layouts in this map and store the hashes, CSS and
	sourceToDest: map[string]string{},
}

// Set ok
func Set(k, sourceFile string, v *FData) {

	mapFS.mu.Lock()
	defer mapFS.mu.Unlock()
	k = filepath.Clean(k)
	// always reset
	v.Processed = false

	// if there is a source file then add the mapping.
	if sourceFile != "" {
		mapFS.sourceToDest[sourceFile] = k
	}

	// same as if v, ok := as no bytes == never seen
	if v, ok := mapFS.fs[k]; ok {
		// has a hash i.e not the zero val and no change in hash == same so no need to gopack etc,,,
		if v.Hash > 0 && v.Hash == CRC32Hasher(v.B) {
			v.Processed = true
		}

	} else { // first time seeing
		setEntry(k, mapFS.entries)
	}
	mapFS.fs[k] = v

}

// Get ok
func Get(k string) *FData {
	return mapFS.fs[filepath.Clean(k)]
}

// Exists ok
func Exists(k string) bool {
	if _, ok := mapFS.fs[filepath.Clean(k)]; ok {
		return true
	}

	return false
}

// GetOrSet wil return existing or new "empty" FData. Used for the layouts  now.
func GetOrSet(k string) *FData {
	mapFS.mu.Lock()
	defer mapFS.mu.Unlock()
	return getOrSet(k, mapFS.fs)
}

func getOrSet(k string, fs map[string]*FData) *FData {
	clean := filepath.Clean(k)
	if v, ok := fs[clean]; ok {
		v.Processed = false
		return v
	}
	d := &FData{}
	fs[clean] = d

	return d
}

// Remove keeps entries and fs in sync
func Remove(k string) {
	mapFS.mu.Lock()
	remove(k, mapFS.fs, mapFS.sourceToDest, mapFS.entries)
	mapFS.mu.Unlock()

}
func remove(k string, fs map[string]*FData, sm map[string]string, entries *[]string) {
	k = filepath.Clean(k)
	// if there is a mapping we need to remove
	if m, ok := sm[k]; ok {
		// from fs
		delete(fs, m)
		// from ordered slice
		deleteEntry(m, entries)
		// from source mapping
		delete(sm, k)
	}

}

// Entries ok
func Entries() *[]string {
	return mapFS.entries
}

// BinSearchIndex finds where the element exists or where it would be inserted to keep slice sorted
func BinSearchIndex(x string) int {

	return binSearchIndex(x, *mapFS.entries)
}

func binSearchIndex(x string, a []string) int {
	return sort.Search(len(a), func(i int) bool {
		return x <= a[i]
	})
}

// lowest order by path and highest order of that dir wins
func sortByDir(eles []string) {
	if len(eles) == 1 {
		return
	}
	sort.SliceStable(eles, func(i, j int) bool {
		// keeps the dir files together
		a, b := eles[i], eles[j]
		cnta, cntb := strings.Count(a, "/"), strings.Count(b, "/")
		// if the same keep the last one which is what happens in findFile regular readdir....
		if cnta == cntb {
			// make same dir files come last
			if filepath.Dir(a) == filepath.Dir(b) {
				return a > b
			}
			return a < b
		}
		// else use the count of /
		return cnta < cntb

	})
}

// adds the element and keeps sorted, O(log n) where to place and constant time insert bar we hit cap and resize?
// https://github.com/golang/go/wiki/SliceTricks
func setEntry(x string, entries *[]string) {
	if len(*entries) == 0 {
		*entries = append(*entries, x)

		return
	}
	i := binSearchIndex(x, *entries)
	*entries = append(*entries, "")
	copy((*entries)[i+1:], (*entries)[i:])
	(*entries)[i] = x

}

// where x it would go/start in sorted array or -1 if it would be last...
func getEntryIndex(x string, entries *[]string) int {
	x = filepath.Clean(x)
	i := binSearchIndex(x, *entries)
	if i == len(*entries) {
		return -1
	}

	return i

}

// log n search to start point, might just be as well slicing...
func StartFrom(x string) <-chan string {
	return startFrom(x, mapFS.entries)

}

func startFrom(x string, entries *[]string) <-chan string {
	i := binSearchIndex(x, *entries)
	if i == len(*entries) {
		return nil
	}

	ch := make(chan string)
	go func() {
		for ; i < len(*entries); i++ {
			ch <- (*entries)[i]
		}
		close(ch)
	}()
	return ch

}

func deleteEntry(x string, entries *[]string) {
	i := getEntryIndex(x, entries)
	log.Println(i, x)
	if i != -1 && (*entries)[i] == x {
		*entries = append((*entries)[:i], (*entries)[i+1:]...)
	}

}

// SearchPath starts at path and looks for .js|.mjs. Need the error so we stop the build if filecannot be found..
func SearchPath(path string) (string, error) {
	return searchPath(path, mapFS.entries)
}

func searchPath(path string, entries *[]string) (string, error) {

	out := []string{}
	for entry := range startFrom(path, entries) {
		// another dir/path beyond what our current "dir(s)" of interest
		if !(strings.HasPrefix(entry, path)) {
			break
		}
		if strings.HasSuffix(entry, ".js") || strings.HasSuffix(entry, ".mjs") {
			out = append(out, entry)

		}

	}

	if len(out) > 0 {

		// This can break as can the ReadDir logic in findJSFile.
		// Need set more logic, maybe hierarchical order like path/index.js/path/index.mjs ...
		sortByDir(out)
		return out[0], nil
	}
	return "", fmt.Errorf("Could not find file %s%s\n", path, Caller())
}

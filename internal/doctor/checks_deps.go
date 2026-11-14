package doctor

import (
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/toml"
)

// checkRegisteredDeps warns about a [[deps]] entry that has no copy under
// ai-specs/.deps/<id>: sync has not vendored it (a failed add, an interrupted
// sync), and nothing else in the report would show it. It is silent for an
// installed dep so healthy projects gain no extra line, and it never joins an
// id that could leave the managed tree into a path.
func (d *Doctor) checkRegisteredDeps() {
	data := d.manifestData()
	if data == nil {
		return
	}
	raw, ok := data.Get("deps")
	if !ok {
		return
	}
	// [[deps]] parses as a list of tables; an inline array arrives as []any.
	var tables []*toml.Table
	switch x := raw.(type) {
	case []*toml.Table:
		tables = x
	case []any:
		for _, item := range x {
			if tbl, ok := item.(*toml.Table); ok {
				tables = append(tables, tbl)
			}
		}
	default:
		return
	}
	for _, tbl := range tables {
		v, _ := tbl.Get("id")
		id, ok := v.(string)
		if !ok || id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
			continue
		}
		if isDir(filepath.Join(d.Root, "ai-specs", ".deps", id)) {
			continue
		}
		d.add(WARN, "deps", "dep '"+id+"' is registered but not installed",
			"ai-specs sync (vendors registered deps into ai-specs/.deps)")
	}
}

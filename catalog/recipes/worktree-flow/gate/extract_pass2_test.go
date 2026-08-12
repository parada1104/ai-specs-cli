package main

import "testing"

// TestExtractPass2PythonOpen covers the Python open(path, mode) family: the
// write-mode check (mode contains w/a/x), the paired-delimiter requirement for
// both path and mode, and the reference's mode semantics (no default-write).
func TestExtractPass2PythonOpen(t *testing.T) {
	cases := []struct {
		name, cmd string
		want      []string
	}{
		{"w mode", `open("out.txt", "w")`, []string{"out.txt"}},
		{"a mode", `open('log', 'a')`, []string{"log"}},
		{"x mode", `open("new.dat", "x")`, []string{"new.dat"}},
		{"single-quoted path", `open('out.txt', "w")`, []string{"out.txt"}},
		{"leading space after paren", `open(  "out.txt", "w")`, []string{"out.txt"}},
		{"spaces before mode", `open("out.txt",    "w")`, []string{"out.txt"}},
		{"mode inside larger string", `open("out.txt", "r+wa")`, []string{"out.txt"}},
		{"embedded slash", `open("sub/dir/file.txt", "w")`, []string{"sub/dir/file.txt"}},
		{"read mode skipped", `open("out.txt", "r")`, nil},
		{"no mode argument", `open("out.txt")`, nil},
		{"read-only flags", `open("out.txt", "rb")`, nil},
		{"mismatched path delimiters", `open("out.txt', 'w')`, nil},
		{"mismatched mode delimiters", `open("out.txt", 'w")`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPass2(tc.cmd)
			if !equalStrings(got, tc.want) {
				t.Fatalf("cmd %q: got %#v want %#v", tc.cmd, got, tc.want)
			}
		})
	}
}

// TestExtractPass2PathWrite covers Path(...).write_text / write_bytes.
func TestExtractPass2PathWrite(t *testing.T) {
	cases := []struct {
		name, cmd string
		want      []string
	}{
		{"write_text", `Path("data.txt").write_text("x")`, []string{"data.txt"}},
		{"write_bytes", `Path('blob.bin').write_bytes(b"x")`, []string{"blob.bin"}},
		{"single-quoted", `Path('data.txt').write_text("x")`, []string{"data.txt"}},
		{"trailing space before paren", `Path("data.txt" ).write_text("x")`, []string{"data.txt"}},
		{"nested path", `Path("a/b/c.txt").write_text("x")`, []string{"a/b/c.txt"}},
		{"mismatched delimiters", `Path("data.txt').write_text("x")`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPass2(tc.cmd)
			if !equalStrings(got, tc.want) {
				t.Fatalf("cmd %q: got %#v want %#v", tc.cmd, got, tc.want)
			}
		})
	}
}

// TestExtractPass2NodeWriters covers the five Node fs writers with and without
// the fs. prefix.
func TestExtractPass2NodeWriters(t *testing.T) {
	cases := []struct {
		name, cmd string
		want      []string
	}{
		{"writeFileSync", `fs.writeFileSync("out.js", data)`, []string{"out.js"}},
		{"writeFileSync bare", `writeFileSync('out.js', data)`, []string{"out.js"}},
		{"appendFileSync", `fs.appendFileSync("log.txt", line)`, []string{"log.txt"}},
		{"writeFile", `fs.writeFile("out.js", data)`, []string{"out.js"}},
		{"appendFile", `fs.appendFile("log.txt", line)`, []string{"log.txt"}},
		{"createWriteStream", `fs.createWriteStream("stream.out")`, []string{"stream.out"}},
		{"embedded slash", `fs.writeFileSync("src/gen.txt", data)`, []string{"src/gen.txt"}},
		{"mismatched delimiters", `fs.writeFileSync("out.js', data)`, nil},
		{"not a write method", `fs.readFileSync("out.js")`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPass2(tc.cmd)
			if !equalStrings(got, tc.want) {
				t.Fatalf("cmd %q: got %#v want %#v", tc.cmd, got, tc.want)
			}
		})
	}
}

// TestExtractPass2RubyFileWrite covers Ruby File.write, which is always a write.
func TestExtractPass2RubyFileWrite(t *testing.T) {
	cases := []struct {
		name, cmd string
		want      []string
	}{
		{"double-quoted", `File.write("out.rb", src)`, []string{"out.rb"}},
		{"single-quoted", `File.write('out.rb', src)`, []string{"out.rb"}},
		{"embedded slash", `File.write("lib/out.rb", src)`, []string{"lib/out.rb"}},
		{"mismatched delimiters", `File.write("out.rb', src)`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPass2(tc.cmd)
			if !equalStrings(got, tc.want) {
				t.Fatalf("cmd %q: got %#v want %#v", tc.cmd, got, tc.want)
			}
		})
	}
}

// TestExtractPass2RubyFileOpen covers Ruby File.open(path, mode): the write
// mode check and both paired-delimiter requirements.
func TestExtractPass2RubyFileOpen(t *testing.T) {
	cases := []struct {
		name, cmd string
		want      []string
	}{
		// The write-mode families run against the raw command string, so the
		// unanchored Python open(path, mode) family also matches the open( call
		// inside File.open( — the reference implementation yields the path
		// twice, and we reproduce that exactly (dedupe happens later, on the
		// combined pass1+pass2 list).
		{"w mode", `File.open("out.rb", "w")`, []string{"out.rb", "out.rb"}},
		{"a mode", `File.open('log', 'a')`, []string{"log", "log"}},
		{"x mode", `File.open("new.dat", "x")`, []string{"new.dat", "new.dat"}},
		{"read mode skipped", `File.open("out.rb", "r")`, nil},
		{"no mode argument", `File.open("out.rb")`, nil},
		{"mismatched path delimiters", `File.open("out.rb', 'w')`, nil},
		{"mismatched mode delimiters", `File.open("out.rb", 'w")`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPass2(tc.cmd)
			if !equalStrings(got, tc.want) {
				t.Fatalf("cmd %q: got %#v want %#v", tc.cmd, got, tc.want)
			}
		})
	}
}

// TestExtractPass2CombinedCommand proves multiple families are found in a
// single command string, in source order.
func TestExtractPass2CombinedCommand(t *testing.T) {
	cmd := `python3 -c 'open("a.txt", "w")'; node -e 'fs.writeFileSync("b.txt", x)'; ruby -e 'File.write("c.rb", x)'`
	got := extractPass2(cmd)
	want := []string{"a.txt", "b.txt", "c.rb"}
	if !equalStrings(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

// TestExtractPass2Scrub pins the reference dedupe() scrub step on the pass2
// families: interpreter writers targeting ".", "-", "&"-prefixed tokens or the
// /dev/null | /dev/stdout | /dev/stderr | /dev/fd/* special files are never
// write candidates. The reference applies scrub in dedupe(pass1(cmd) +
// pass2(cmd)) (worktree-gate-legacy.sh:90-100, 274), so the pass2 output is
// asserted through the same dedupeStrings wrapper the event path uses.
func TestExtractPass2Scrub(t *testing.T) {
	cases := []struct {
		name, cmd string
		want      []string
	}{
		{"python open dot", `open(".", "w")`, nil},
		{"python open dash", `open('-', 'w')`, nil},
		{"python open amp-star", `open("&*", "w")`, nil},
		{"python open dev-null", `open("/dev/null", "w")`, nil},
		{"python open dev-stdout", `open("/dev/stdout", "w")`, nil},
		{"python open dev-stderr", `open("/dev/stderr", "w")`, nil},
		{"python open dev-fd", `open("/dev/fd/2", "w")`, nil},
		{"path write_text dot", `Path(".").write_text("x")`, nil},
		{"path write_bytes dash", `Path('-').write_bytes(b"x")`, nil},
		{"path write_text dev-null", `Path("/dev/null").write_text("x")`, nil},
		{"node writeFileSync dev-null", `fs.writeFileSync("/dev/null", x)`, nil},
		{"node writeFileSync dot", `writeFileSync('.', x)`, nil},
		{"node createWriteStream dev-fd", `fs.createWriteStream("/dev/fd/1")`, nil},
		{"ruby File.write dash", `File.write('-', s)`, nil},
		{"ruby File.write dev-null", `File.write("/dev/null", s)`, nil},
		{"ruby File.open dev-stdout", `File.open("/dev/stdout", "w")`, nil},
		{"ruby File.open dot", `File.open(".", "w")`, nil},
		{"real python path kept", `open("out.txt", "w")`, []string{"out.txt"}},
		{"real node path kept", `fs.writeFileSync("out.js", x)`, []string{"out.js"}},
		{"real ruby path kept", `File.write("out.rb", s)`, []string{"out.rb"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dedupeStrings(extractPass2(tc.cmd))
			if !equalStrings(got, tc.want) {
				t.Fatalf("cmd %q: got %#v want %#v", tc.cmd, got, tc.want)
			}
		})
	}
}

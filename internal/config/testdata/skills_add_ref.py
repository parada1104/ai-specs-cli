# TEST-ONLY verbatim replica of the python3 heredoc in lib/skills-add.sh
# (extracted unchanged; argv-based). Do not modify — differential tests
# compare Go behavior against this exact logic. lib/ itself is untouched.
import sys, pathlib

toml_path, dep_id, url, subdir, scope_csv, trigger, license_, attribution, ref = sys.argv[1:10]

def s(x: str) -> str:
    return '"' + x.replace("\\", "\\\\").replace('"', '\\"') + '"'

scopes = [x.strip() for x in scope_csv.split(",") if x.strip()] or ["root"]

block = ["", "[[deps]]"]
block.append(f"id = {s(dep_id)}")
block.append(f"source = {s(url)}")
if subdir:
    block.append(f"path = {s(subdir)}")
block.append("scope = [" + ", ".join(s(x) for x in scopes) + "]")
if trigger:
    block.append(f"auto_invoke = [{s(trigger)}]")
if license_:
    block.append(f"license = {s(license_)}")
if attribution:
    block.append(f"vendor_attribution = {s(attribution)}")
if ref:
    block.append(f"ref = {s(ref)}")
block.append("")

p = pathlib.Path(toml_path)
content = p.read_text()
if not content.endswith("\n"):
    content += "\n"
p.write_text(content + "\n".join(block))
print(f"  ✓ appended [[deps]] for '{dep_id}' to {toml_path}")

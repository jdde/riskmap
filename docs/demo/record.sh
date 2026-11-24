#!/bin/sh
# Re-creates docs/demo.gif. Run from the repository root:
#     sh docs/demo/record.sh
# Needs: riskmap on the PATH, git, python3 with Pillow, macOS system fonts.
# The script replays a real session in a copy of tests/examples/nextjs-web and
# hands the captured terminal output to render.py.
set -eu
root=$(pwd)
work=$(mktemp -d)
repo="$work/repo"
mkdir -p "$repo"
(cd tests/examples/nextjs-web && tar cf - --exclude .riskmap.yml .) | tar xf - -C "$repo"
cd "$repo"
commit() { git -c user.name=demo -c user.email=demo@example.com commit -qm "$1"; }
git init -q && git add -A && commit init

cat > .riskmap.yml <<'YAML'
version: 1

default: { risk: high }        # no rule matches? treat it as risky

rules:
  - id: docs
    match: ["docs/**", "README.md"]
    labels: { risk: low }
    reason: "Documentation, no effect at runtime"

  - id: ui
    match: ["src/components/**", "src/app/**/page.tsx"]
    labels: { risk: medium }
    reason: "Visible UI, mistakes show up in the browser"

  - id: ci
    match: [".github/**"]
    labels: { risk: high }
    reason: "CI and gates must not loosen themselves"
YAML
riskmap validate --format text > "$work/out1.txt" 2>&1
git add -A && commit "add riskmap"

sed -i.bak 's|<button type={type}>|<button type={type} className="btn">|' src/components/Button.tsx
rm src/components/Button.tsx.bak
git add -A
riskmap check --staged --format text > "$work/out2.txt" 2>&1

sed -i.bak 's|- run: npm test$|- run: npm test \|\| true|' .github/workflows/ci.yml
rm .github/workflows/ci.yml.bak
git add -A
riskmap check --staged --format text > "$work/out3.txt" 2>&1 || true

python3 "$root/docs/demo/render.py" "$work" "$root/docs/demo.gif"
rm -rf "$work"

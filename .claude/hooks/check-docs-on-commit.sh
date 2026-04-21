#!/bin/bash
#
# Claude Code Hook: Check whether docs are updated on feat commits
#
# When source code is changed in a feature (feat) commit
# but the docs/ directory has no changes, the commit is blocked.
# Non-feature commits (fix, refactor, test, chore, etc.) are allowed through.
#

# Read hook input JSON from stdin
INPUT=$(cat)

# Extract Bash command
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // ""')

# Allow through if not a git commit command
if ! echo "$COMMAND" | grep -qE 'git\s+commit'; then
  exit 0
fi

# Check if commit message starts with feat
# Claude Code commit command formats:
#   git commit -m "feat: ..."
#   git commit -m "$(cat <<'EOF'\nfeat: ...\nEOF\n)"
# Determine by checking for feat keyword in the full command
IS_FEAT=false
if echo "$COMMAND" | grep -qiE '(^|[\s"'"'"'(])feat[\s(:]'; then
  IS_FEAT=true
fi

# Allow through if not a feat commit (fix, refactor, test, chore, docs, ci, etc.)
if [ "$IS_FEAT" = false ]; then
  exit 0
fi

# Get list of staged files
STAGED_FILES=$(git diff --cached --name-only 2>/dev/null || true)

# Allow through if no staged files (commit will fail anyway)
if [ -z "$STAGED_FILES" ]; then
  exit 0
fi

# Source code change detection patterns
# Go source, CRD YAML, config/samples, Makefile, etc.
SOURCE_PATTERNS="\.go$|config/crd/|config/samples/|config/rbac/|Makefile|\.yaml$|\.yml$"

# Docs change detection pattern
DOCS_PATTERN="^docs/"

# Check for source code changes
HAS_SOURCE_CHANGES=false
while IFS= read -r file; do
  if echo "$file" | grep -qE "$SOURCE_PATTERNS"; then
    # Exclude files inside docs/
    if ! echo "$file" | grep -qE "$DOCS_PATTERN"; then
      HAS_SOURCE_CHANGES=true
      break
    fi
  fi
done <<< "$STAGED_FILES"

# Allow through if no source code changes (docs-only changes, README edits, etc.)
if [ "$HAS_SOURCE_CHANGES" = false ]; then
  exit 0
fi

# Check for docs changes
HAS_DOCS_CHANGES=false
while IFS= read -r file; do
  if echo "$file" | grep -qE "$DOCS_PATTERN"; then
    HAS_DOCS_CHANGES=true
    break
  fi
done <<< "$STAGED_FILES"

# Block if source changes exist but no docs changes
if [ "$HAS_DOCS_CHANGES" = false ]; then
  # Build list of changed source files
  CHANGED_SOURCE_FILES=$(echo "$STAGED_FILES" | grep -E "$SOURCE_PATTERNS" | grep -vE "$DOCS_PATTERN" || true)

  echo "feat commit: Source code was changed but the docs/ directory has no changes." >&2
  echo "" >&2
  echo "Changed source files:" >&2
  while IFS= read -r f; do
    [ -n "$f" ] && echo "  - $f" >&2
  done <<< "$CHANGED_SOURCE_FILES"
  echo "" >&2
  echo "Please update the documentation in the docs/ directory and commit again." >&2
  echo "Docusaurus docs location: docs/content/" >&2
  exit 2
fi

exit 0

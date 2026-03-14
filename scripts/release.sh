#!/bin/bash
set -e

# Build mayo first to ensure we have the latest generator
make build > /dev/null

# Get current version from pkg/version/version.go
CURRENT_VERSION=$(grep -oE "[0-9]+\.[0-9]+\.[0-9]+" pkg/version/version.go)
echo "Current version: $CURRENT_VERSION"
read -p "Enter new version (e.g. 1.2.1): " NEW_VERSION

if [ -z "$NEW_VERSION" ]; then
    echo "Version cannot be empty."
    exit 1
fi

# Update version file
sed -i '' "s/Version = \".*\"/Version = \"$NEW_VERSION\"/" pkg/version/version.go

# Create changelog file
CHANGELOG_FILE="internal/changelog/data/v$NEW_VERSION.md"

echo "🤖 Generating AI changelog from commits..."
# Use the newly built mayo to generate release notes
# We extract the part after the separator
bin/mayo release-notes "v$NEW_VERSION" | sed -n '/--- GENERATED CHANGELOG ---/,/---------------------------/p' | sed '1d;$d' > "$CHANGELOG_FILE"

echo "--- Preview of generated changelog ---"
cat "$CHANGELOG_FILE"
echo "--------------------------------------"

read -p "Do you want to edit this changelog? (y/n): " EDIT_CHOICE
if [ "$EDIT_CHOICE" = "y" ]; then
    ${EDITOR:-nano} "$CHANGELOG_FILE"
fi

# Commit and Tag
echo "Committing and tagging..."
git add pkg/version/version.go "$CHANGELOG_FILE"
git commit -m "chore: release v$NEW_VERSION"
git tag -a "v$NEW_VERSION" -m "Release v$NEW_VERSION"

# Build Artifact (Final build with embedded changelog)
echo "Building final artifact with embedded changelog..."
make build
mkdir -p releases
tar -czvf "releases/mayo-v$NEW_VERSION-mac.tar.gz" -C bin mayo

echo ""
echo "✅ Release v$NEW_VERSION prepared with AI!"
echo "1. Git artifact created: releases/mayo-v$NEW_VERSION-mac.tar.gz"
echo "2. Version bumped to $NEW_VERSION"
echo "3. AI Changelog saved at $CHANGELOG_FILE"
echo ""
echo "Next steps: git push origin main --tags"

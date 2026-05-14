#!/usr/bin/env fish

set repo_parent /worker/work/repo
set repo_name aws-demo
set repo_slug england2/aws-demo
set sparse_path test-applications

mkdir -p "$repo_parent"; or exit 1
cd "$repo_parent"; or exit 1

if test -e "$repo_name"
    echo "error: $repo_parent/$repo_name already exists" >&2
    exit 1
end

gh repo clone "$repo_slug" -- --filter=blob:none --sparse; or exit 1

cd "$repo_parent/$repo_name"; or exit 1

git sparse-checkout set --cone "$sparse_path"; or exit 1

echo "sparse checkout configured:"
git sparse-checkout list; or exit 1

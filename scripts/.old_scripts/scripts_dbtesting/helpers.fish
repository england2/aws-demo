# in other fish scripts next to helper.fish:
# source (dirname (realpath (status filename)))/helpers.fish

function cdgit
    while not test -d .git
        if test (pwd) = /
            echo "error: no git repository found in parent directories."
            exit 1
        end
        cd ..
    end
end

function repo_root
    set -l original_pwd (pwd)
    cdgit
    set -l found_root (pwd)
    cd "$original_pwd"
    echo "$found_root"
end

# repo_root env-var is accessible to all scripts that import helpers.fish
set -gx repo_root (repo_root)

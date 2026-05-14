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

# repo root env var is accessible to all fish scripts in ./scripts
set -gx repo_root (repo_root)

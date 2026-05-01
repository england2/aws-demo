#!/bin/fish

set script_dir (cd (dirname (status --current-filename)); pwd)
set remote "admin@35.167.192.31"
set key "~/downloads/aws-demo-key.pem"
set remote_dir "/home/admin/"

rsync -av \
    --exclude "copy-inside.fish" \
    -e "ssh -i $key" \
    "$script_dir"/ \
    "$remote:$remote_dir"

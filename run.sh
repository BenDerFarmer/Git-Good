go build .
sudo setcap 'cap_net_bind_service=+ep' git-good

./git-good

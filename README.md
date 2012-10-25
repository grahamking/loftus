Attempt to store my dotfiles etc in a git repo, and make a Go daemon that manages it for me, using inotify.

NOT READY YET.

## Working Notes

**Git file metadata:**

  - Does exec bit correctly
  - Doesn't preserve owner / group. No problem.
  - Doesn't preserve permissions: 600 file comes back as whatever umask is

Only problem is probably ssh private key: `.ssh/id_dsa or id_rsa`
Can save acl with `getfacl` into a separate file, then restore it with `setfacl --restore=`.

**Do we get told when machine is shutting down / we are stopping?**
SIGTERM, need to trap it with os/signal
http://stackoverflow.com/a/11269077/146620

**Initial setup**
On server:

    sudo adduser loftus --shell /usr/bin/git-shell --disabled-password
    sudo mkdir --mode=777 /home/loftus/.ssh

    cd /home/loftus/
    mkdir repo.git ; cd repo
    git init --bare .
    /usr/local/bin/loftus --server --address=my.example.com:8007

On client:

    ssh-keygen -f ~/.ssh/id_rsa.loftus    # Do not add a passphrase - just hit enter
    scp ~/.ssh/id_rsa.loftus.pub my.example.com:/home/loftus/.ssh/authorized_keys

    Edit .ssh/config to include something like:

        Host loftus_server
            HostName my.example.com
            User loftus
            IdentityFile ~/.ssh/id_rsa.loftus

On server:
    sudo chmod -R 500 ~loftus/.ssh
    sudo chown -R loftus:loftus ~loftus/.ssh

## Add client

    git clone ssh://loftus_server/~/repo.git loftus    # See .ssh/config earlier
    /usr/local/loftus --address=my.example.com:8007

## Upstart

Copy `loftus` to `/usr/local/bin`. Also create `/usr/local/bin/loftus_info` and `/usr/local/bin/loftus_alert` (program gives suggestions for contents on startup).

Copy the example `loftus.conf` into `/etc/init/loftus.conf`. Be sure to change all instances of 'graham' to your username.

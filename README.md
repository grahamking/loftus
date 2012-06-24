
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
Probably a sigkill - trap it and run two git commands.

**How to start it?**
Need to get ssh key password from user.
Maybe run from window manager startup apps, not as daemon.
Need solid ssh-agent setup.
Too many retries triggers denyhosts to lock me out!

**Other option is loftus specific user, ssh key with no passphrase**

On server:

    sudo adduser loftus --shell /usr/bin/git-shell --disabled-password
    sudo mkdir --mode=777 /home/loftus/.ssh

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

## Setup

Server (as user loftus):    # but loftus has no shell!?

    cd /home/loftus/
    mkdir repo.git ; cd repo
    git init --bare .
    /usr/local/bin/loftus --server --address=daisy.gkgk.org:8007

Client:

    git init .                       # Create local repo
    git remote add origin ssh://...  # Declare where master is
                                     # ssh://loftus@server.example.com/~loftus/repo.git
    git pull origin master           # Fill directory
    git push -u origin master        # So that bare "git pull" works
    /usr/local/loftus --address=daisy.gkgk.org:8007

Client alternative (isn't this better than above?)

    git clone ssh://loftus_server/~/loftus.git    # See .ssh/config earlier


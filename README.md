
Attempt to store my dotfiles etc in a git repo, and make a Go daemon that manages it for me, using inotify. NOT READY YET.

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
Too many retries triggers denyhosts to lock me out!

## Setup

Server:

    mkdir repo.git ; cd repo
    git init --bare .
    /usr/local/bup --server --address=daisy.gkgk.org:8007

Client:

    git init .                       # Create local repo
    git remote add origin ssh://...  # Declare where master is
                                    # ssh://graham@server.example.com/~graham/repo.git
    git pull origin master           # Fill directory
    git push -u origin master        # So that bare "git pull" works
    /usr/local/bup --address=daisy.gkgk.org:8007

Client alternative (isn't this better than above?)

    git clone ssh://username@example.com/~/loftus.git


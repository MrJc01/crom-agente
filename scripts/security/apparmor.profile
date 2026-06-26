#include <tunables/global>

/usr/local/bin/crom-agente {
  # Include base abstractions
  #include <abstractions/base>
  #include <abstractions/nameservice>
  #include <abstractions/user-tmp>

  # Network access for API calls (OpenRouter, OpenAI, etc.)
  network inet stream,
  network inet6 stream,

  # Read access to all system binaries and libraries
  /lib/** rm,
  /usr/lib/** rm,
  /usr/bin/** rix,
  /bin/** rix,

  # Workspace access (assuming it runs in the user's home directory or specific paths)
  owner @{HOME}/** rwk,
  owner /tmp/** rwk,

  # Explicitly deny write access to critical system directories
  deny /etc/** w,
  deny /var/** w,
  deny /usr/** w,
  deny /bin/** w,
  deny /sbin/** w,
  deny /lib/** w,
  deny /sys/** w,
  deny /boot/** w,

  # Allow execution of bubblewrap (bwrap) for sandboxing if installed
  /usr/bin/bwrap Px,
}

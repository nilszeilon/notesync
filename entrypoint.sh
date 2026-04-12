#!/bin/sh
# If PUID/PGID are set, create a user with those IDs and run as that user.
# Otherwise, run as whoever the container defaults to (root).

if [ -n "$PUID" ] && [ -n "$PGID" ]; then
  groupadd -g "$PGID" -o notesync 2>/dev/null || true
  useradd -u "$PUID" -g "$PGID" -o -M -s /bin/sh notesync 2>/dev/null || true

  # Ensure mounted volumes are accessible
  for dir in /data /_site /notes; do
    [ -d "$dir" ] && chown -R "$PUID:$PGID" "$dir"
  done

  exec gosu notesync "$@"
fi

exec "$@"

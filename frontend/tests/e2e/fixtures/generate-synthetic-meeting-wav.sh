#!/usr/bin/env bash
set -euo pipefail

# Builds one harmless, offline spoken WAV for the optional VM acceptance run.
# It deliberately has no user or customer data.  The speech engine and ffmpeg
# must already be installed on the VM; this script never downloads either.

output=
dry_run=false

while (($#)); do
  case "$1" in
    --output)
      output=${2:?--output needs a path}
      shift 2
      ;;
    --dry-run)
      dry_run=true
      shift
      ;;
    *)
      echo "usage: $0 --output PATH [--dry-run]" >&2
      exit 2
      ;;
  esac
done

if [[ -z "$output" ]]; then
  echo "usage: $0 --output PATH [--dry-run]" >&2
  exit 2
fi

if "$dry_run"; then
  cat <<EOF
offline fixture: $output
speech input: synthetic meeting only; no customer data
commands: espeak-ng --stdout --stdin | ffmpeg -f wav -i pipe:0 -ac 1 -ar 16000 -c:a pcm_s16le
browser upload: POST /api/v1/media/upload multipart file=$output
only after separate approval: POST /api/v1/transcriptions with the uploaded media id
cleanup after provider acceptance: DELETE /api/v1/transcriptions/:id then DELETE /api/v1/media/:id
EOF
  exit 0
fi

command -v espeak-ng >/dev/null || { echo "espeak-ng is required on the acceptance VM" >&2; exit 1; }
command -v ffmpeg >/dev/null || { echo "ffmpeg is required on the acceptance VM" >&2; exit 1; }
command -v ffprobe >/dev/null || { echo "ffprobe is required on the acceptance VM" >&2; exit 1; }
command -v sha256sum >/dev/null || { echo "sha256sum is required on the acceptance VM" >&2; exit 1; }

script=$(mktemp)
trap 'rm -f "$script"' EXIT
cat >"$script" <<'TEXT'
This is a synthetic Voxis OSS acceptance meeting. Nia opens the review on October fifteenth, twenty twenty-six. Rafi will complete the data protection assessment by October twenty-first. Elena says the fourth quarter budget is uncertain. Nia approves a limited pilot. Nia will publish the retention schedule by October twenty-third. The two decisions are the limited pilot and the retention schedule. This recording contains no customer or personal data.
TEXT

mkdir -p "$(dirname "$output")"
espeak-ng --stdout --stdin <"$script" |
  ffmpeg -hide_banner -loglevel error -y -f wav -i pipe:0 -ac 1 -ar 16000 -c:a pcm_s16le "$output"

duration=$(ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 "$output")
awk -v seconds="$duration" 'BEGIN { exit !(seconds > 0 && seconds <= 60) }' || {
  echo "synthetic WAV duration must be in (0, 60] seconds" >&2
  exit 1
}

hash=$(sha256sum "$output" | awk '{print $1}')
printf '{"path":"%s","sha256":"%s","duration_seconds":%s}\n' "$output" "$hash" "$duration"

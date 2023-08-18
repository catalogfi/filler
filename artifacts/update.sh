#!/bin/sh
main(){

  # Check if cobi has been installed
  if ! check_cmd cobi; then
    echo "cannot find cobi"
    err "please install cobi first"
  fi

  echo "Updating cobi ..."

  progressBar 20 100

  # Update the binary
  current=$(cobi --version | grep "COBI version" | cut -d ' ' -f 4)
  latest=$(get_latest_release)
  vercomp $current $latest
  if [ "$?" -eq "2" ]; then
    ostype="$(uname -s | tr '[:upper:]' '[:lower:]')"
    cputype="$(uname -m | tr '[:upper:]' '[:lower:]')"
    if [ $cputype = "x86_64" ];then
      cputype="amd64"
    fi
    progressBar 30 100

    cobi_url="https://cobi-releases.s3.ap-south-1.amazonaws.com/cobi_${ostype}_${cputype}"
    ensure downloader "$cobi_url" "$HOME/.catalog/bin/catalog"
    ensure chmod +x "$HOME/.catalog/bin/catalog"

    progressBar 100 100
    sleep 1
    echo ''
    echo "Done! Your 'cobi' has been updated to $latest."
  else
    progressBar 100 100
    echo ''
    echo "You're running the latest version"
  fi
}

# Source: https://sh.rustup.rs
check_cmd() {
    command -v "$1" > /dev/null 2>&1
}

# This wraps curl or wget. Try curl first, if not installed, use wget instead.
# Source: https://sh.rustup.rs
downloader() {
    if check_cmd curl; then
        if ! check_help_for curl --proto --tlsv1.2; then
            curl --silent --show-error --fail --location "$1" --output "$2"
        else
            curl --proto '=https' --tlsv1.2 --silent --show-error --fail --location "$1" --output "$2"
        fi
    elif check_cmd wget; then
        if ! check_help_for wget --https-only --secure-protocol; then
            wget "$1" -O "$2"
        else
            wget --https-only --secure-protocol=TLSv1_2 "$1" -O "$2"
        fi
    else
        echo "Unknown downloader"   # should not reach here
    fi
}

# Source: https://sh.rustup.rs
check_help_for() {
    local _cmd
    local _arg
    local _ok
    _cmd="$1"
    _ok="y"
    shift

    for _arg in "$@"; do
        if ! "$_cmd" --help | grep -q -- "$_arg"; then
            _ok="n"
        fi
    done

    test "$_ok" = "y"
}

# Source: https://sh.rustup.rs
err() {
    echo ''
    echo "$1" >&2
    exit 1
}

# Source: https://sh.rustup.rs
ensure() {
    if ! "$@"; then err "command failed: $*"; fi
}

get_latest_release() {
  curl --silent "https://cobi-releases.s3.ap-south-1.amazonaws.com/VERSION" # Get latest release from GitHub api
}

vercomp () {
    if [[ $1 == $2 ]]
    then
        return 0
    fi
    major1="$(echo $1 | cut -d. -f1)"
    minor1="$(echo $1 | cut -d. -f2)"
    patch1="$(echo $1 | cut -d. -f3)"
    major2="$(echo $2 | cut -d. -f1)"
    minor2="$(echo $2 | cut -d. -f2)"
    patch2="$(echo $2 | cut -d. -f3)"

    if [ "$major1" -lt "$major2" ]; then
      return 2
    elif [ "$major1" -eq "$major2" ]; then
      if [ "$minor1" -lt "$minor2" ]; then
        return 2
      elif [ "$minor1" -eq "$minor2" ]; then
        if [ "$patch1" -lt "$patch2" ]; then
          return 2
        fi
      fi
    fi

    return 1
}

# Source: https://github.com/fearside/ProgressBar
progressBar() {
    _progress=$1
    _done=$((_progress*5/10))
    _left=$((50-_done))
    done=""
    if ! [ $_done = "0" ];then
        done=$(printf '#%.0s' $(seq $_done))
    fi
    left=""
    if ! [ $_left = "0" ];then
      left=$(printf '=%.0s' $(seq $_left))
    fi
    printf "\rProgress : [$done$left] ${_progress}%%"
}

main "$@" || exit 1

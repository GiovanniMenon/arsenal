# bash hooks: write every command run in a session to its command log.
if [ -z "${_ARS_HOOKS:-}" ]; then
_ARS_HOOKS=1
_ars_id=; _ars_file=; _ars_j=; _ars_t=0; _ars_t0=0; _ars_ready=; _ars_hnum=; _ars_rc=0
_ars_dir=; _ars_name=; _ars_pwd=
_ars_find() {
  local d=$PWD l
  _ars_pwd=$PWD; _ars_dir=; _ars_name=
  while :; do
    if [ -d "$d/.arsenal" ]; then
      _ars_dir=$d
      while IFS= read -r l; do
        case $l in name:*) l=${l#name:}; l=${l//[\"\' ]/}; _ars_name=$l; break ;; esac
      done < "$d/.arsenal/session.yml" 2>/dev/null
      [ -n "$_ars_name" ] || _ars_name=${d##*/}
      [ -e "$d/.arsenal/commands.jsonl" ] || ( umask 077; : >> "$d/.arsenal/commands.jsonl" ) 2>/dev/null
      return 0
    fi
    if [ "$d" = / ] || [ -z "$d" ]; then return 0; fi
    d=${d%/*}; [ -n "$d" ] || d=/
  done
}
: "${_ars_n:=0}"
_ars_ms() {
  if [ "${BASH_VERSINFO[0]}" -ge 5 ]; then
    _ars_t=${EPOCHREALTIME//[.,]/}; _ars_t=${_ars_t:0:13}
  elif [ "${BASH_VERSINFO[0]}" -eq 4 ] && [ "${BASH_VERSINFO[1]}" -ge 2 ]; then
    printf -v _ars_t '%(%s)T' -1; _ars_t=${_ars_t}000
  else
    _ars_t=$(command date +%s)000
  fi
}
_ars_esc() {
  local s=$1
  s=${s//\\/\\\\}
  s=${s//\"/\\\"}
  s=${s//$'\n'/\\n}
  s=${s//$'\t'/\\t}
  s=${s//$'\r'/\\r}
  s=${s//[[:cntrl:]]/ }
  _ars_j=$s
}
_ars_log() { printf '%s\n' "$1" >> "$_ars_file" 2>/dev/null; return 0; }
_ars_dbg() {
  [ -n "$_ars_ready" ] || return 0
  [ -z "${COMP_LINE:-}" ] || return 0
  _ars_ready=
  _ars_id=
  [ "$PWD" = "$_ars_pwd" ] || _ars_find
  local dir=$_ars_dir name=$_ars_name
  if [ -n "${ARSENAL_REC_ID:-}" ] && [ -n "${ARSENAL_SESSION_DIR:-}" ]; then dir=$ARSENAL_SESSION_DIR name=${ARSENAL_SESSION:-}; fi
  [ -n "$dir" ] && [ -z "${ARSENAL_PAUSED:-}" ] && [ -w "$dir/.arsenal/commands.jsonl" ] || return 0
  local h num c trunc= cwd sess rec=
  local HISTTIMEFORMAT=
  h=$(history 1)
  h=${h#"${h%%[![:space:]]*}"}
  num=${h%%[[:space:]]*}
  [ -n "$num" ] || return 0
  [ "$num" != "$_ars_hnum" ] || return 0
  _ars_hnum=$num
  h=${h#"$num"}
  c=${h#  }
  case $c in ' '*) return 0 ;; esac
  if [ "${#c}" -gt 3000 ]; then c=${c:0:3000}; trunc=',"trunc":1'; fi
  _ars_file=$dir/.arsenal/commands.jsonl
  _ars_ms; _ars_t0=$_ars_t
  _ars_n=$((_ars_n + 1)); _ars_id="$$.$_ars_n"
  _ars_esc "$c"; c=$_ars_j
  _ars_esc "$PWD"; cwd=$_ars_j
  _ars_esc "$name"; sess=$_ars_j
  if [ -n "${ARSENAL_REC_ID:-}" ]; then _ars_esc "$ARSENAL_REC_ID"; rec=",\"rec\":\"$_ars_j\""; fi
  _ars_log "{\"v\":1,\"ev\":\"start\",\"id\":\"$_ars_id\",\"ts\":$_ars_t,\"session\":\"$sess\",\"cwd\":\"$cwd\",\"cmd\":\"$c\",\"sh\":\"bash\",\"pid\":$$$rec$trunc}"
  return 0
}
_ars_pc_start() {
  _ars_rc=$?
  if [ -n "$_ars_id" ]; then
    local id=$_ars_id
    _ars_id=
    if [ -w "$_ars_file" ]; then
      _ars_ms
      _ars_log "{\"v\":1,\"ev\":\"end\",\"id\":\"$id\",\"ts\":$_ars_t,\"exit\":$_ars_rc,\"dur\":$((_ars_t - _ars_t0))}"
    fi
  fi
  return $_ars_rc
}
_ars_pc_end() { _ars_ready=1; return $_ars_rc; }
_ars_tf="${TMPDIR:-/tmp}/.ars-trap.$$.$RANDOM"
trap -p DEBUG > "$_ars_tf" 2>/dev/null
_ars_old=$(<"$_ars_tf"); rm -f "$_ars_tf"
_ars_old=${_ars_old#trap -- }; _ars_old=${_ars_old% DEBUG}
if [ -n "$_ars_old" ]; then
  eval "_ars_old=$_ars_old"
  trap '_ars_dbg; '"$_ars_old" DEBUG
else
  trap '_ars_dbg' DEBUG
fi
unset _ars_tf _ars_old
case "$(declare -p PROMPT_COMMAND 2>/dev/null)" in
  "declare -a"*) PROMPT_COMMAND=(_ars_pc_start "${PROMPT_COMMAND[@]}" _ars_pc_end) ;;
  *) PROMPT_COMMAND="_ars_pc_start${PROMPT_COMMAND:+; $PROMPT_COMMAND}; _ars_pc_end" ;;
esac
fi

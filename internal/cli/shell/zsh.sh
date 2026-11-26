# zsh hooks: write every command run in a session to its command log.
zmodload zsh/datetime 2>/dev/null
autoload -Uz add-zsh-hook
typeset -g _ars_id= _ars_file= _ars_j= _ars_dir= _ars_name=
_ars_find() {
  _ars_dir= _ars_name=
  local d=$PWD l
  while true; do
    if [[ -d $d/.arsenal ]]; then
      _ars_dir=$d
      while IFS= read -r l; do
        if [[ $l == name:* ]]; then _ars_name=${${l#name:}//[\"\' ]/}; break; fi
      done < $d/.arsenal/session.yml 2>/dev/null
      [[ -n $_ars_name ]] || _ars_name=${d:t}
      [[ -e $d/.arsenal/commands.jsonl ]] || ( umask 077; : >> $d/.arsenal/commands.jsonl ) 2>/dev/null
      return 0
    fi
    [[ $d == / || -z $d ]] && return 0
    d=${d:h}
  done
}
_ars_find
add-zsh-hook chpwd _ars_find
typeset -gi _ars_n _ars_t0 _ars_now
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
_ars_log() { { print -rn -- "$1"$'\n' >> "$_ars_file" } 2>/dev/null; }
_ars_preexec() {
  _ars_id=
  local dir=$_ars_dir name=$_ars_name
  if [[ -n ${ARSENAL_REC_ID-} && -n ${ARSENAL_SESSION_DIR-} ]]; then dir=$ARSENAL_SESSION_DIR name=${ARSENAL_SESSION-}; fi
  [[ -n $dir && -z ${ARSENAL_PAUSED-} && -w $dir/.arsenal/commands.jsonl ]] || return 0
  [[ $1 == ' '* ]] && return 0
  _ars_file=$dir/.arsenal/commands.jsonl
  _ars_now=$(( EPOCHREALTIME * 1000 ))
  _ars_t0=$_ars_now
  _ars_id="$$.$(( ++_ars_n ))"
  local c=$1 trunc= cwd sess rec=
  if (( ${#c} > 3000 )); then c=${c[1,3000]}; trunc=',"trunc":1'; fi
  _ars_esc "$c"; c=$_ars_j
  _ars_esc "$PWD"; cwd=$_ars_j
  _ars_esc "$name"; sess=$_ars_j
  if [[ -n ${ARSENAL_REC_ID-} ]]; then _ars_esc "$ARSENAL_REC_ID"; rec=",\"rec\":\"$_ars_j\""; fi
  _ars_log "{\"v\":1,\"ev\":\"start\",\"id\":\"$_ars_id\",\"ts\":$_ars_now,\"session\":\"$sess\",\"cwd\":\"$cwd\",\"cmd\":\"$c\",\"sh\":\"zsh\",\"pid\":$$$rec$trunc}"
}
_ars_precmd() {
  local rc=$?
  [[ -n $_ars_id ]] || return $rc
  local id=$_ars_id
  _ars_id=
  [[ -w $_ars_file ]] || return $rc
  _ars_now=$(( EPOCHREALTIME * 1000 ))
  _ars_log "{\"v\":1,\"ev\":\"end\",\"id\":\"$id\",\"ts\":$_ars_now,\"exit\":$rc,\"dur\":$(( _ars_now - _ars_t0 ))}"
  return $rc
}
add-zsh-hook preexec _ars_preexec
add-zsh-hook precmd _ars_precmd
precmd_functions=(_ars_precmd ${precmd_functions:#_ars_precmd})

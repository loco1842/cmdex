# Cmdex shell integration — zsh
#
# This directory is pointed to by $ZDOTDIR (see integrationFor in
# shell_integration.go) so zsh loads these files instead of the user's own
# ~/.zshenv, ~/.zprofile, ~/.zshrc. $ZDOTDIR is read fresh by zsh before each
# of the four startup files, so .zshrc (below) restores it to the user's real
# dotfile directory once their own config has loaded, and .zlogin is picked
# up from there automatically — this file never needs to ship its own
# .zlogin.
#
# .zshenv runs for every zsh invocation, not just interactive login shells
# (plain `zsh -c ...`, a script with a zsh shebang, etc). That's intentional:
# it's also where we install our OSC 133 hooks, and doing so here — before
# the user's own .zshrc has a chance to add its own precmd/preexec hooks
# (oh-my-zsh, prompt themes, ...) — means ours run FIRST each time. That
# matters because $? is only trustworthy for "the command that just
# finished" if nothing else has run a command in between, and other
# precmd hooks (e.g. anything that shells out for git status) can clobber it
# before a later-registered hook gets to read it.

if [ -n "$CMDEX_USER_ZDOTDIR" ] && [ -r "$CMDEX_USER_ZDOTDIR/.zshenv" ]; then
    source "$CMDEX_USER_ZDOTDIR/.zshenv"
fi

autoload -Uz add-zsh-hook

# __cmdex_preexec fires just before a command's output starts, i.e. right as
# the command begins executing.
__cmdex_preexec() {
    print -n '\e]133;C\a'
}

# __cmdex_precmd fires once the command has finished, right before zsh
# redraws the prompt. $? here is the finished command's real exit status —
# reading it as literally the first thing in this function (before any other
# statement can run and change it) is what makes that reliable.
__cmdex_precmd() {
    print -n "\e]133;D;$?\a"
}

add-zsh-hook preexec __cmdex_preexec
add-zsh-hook precmd __cmdex_precmd

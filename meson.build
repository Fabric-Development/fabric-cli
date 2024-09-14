project('fabric-cli', version: '0.0.2', license: 'AGPL-3.0-or-later')

# there's no official GoLang support in meson...
go_cc = find_program('go', required: true)

sources_dir = meson.current_source_dir()
autocompletions_dir = sources_dir / 'autocompletions'

extra_cc_flags = []
if get_option('buildtype') == 'release'
    extra_cc_flags += ['-ldflags=-s -w -compressdwarf=false -linkmode external']
endif


# build (and install) binaries
cli_target = custom_target(
    build_always_stale: true,
    build_by_default: true,
    console: true,
    install: true,
    install_dir: get_option('bindir'),
    output: meson.project_name(),
    command: [go_cc, 'build', '-v'] + extra_cc_flags + ['-o', '@OUTPUT@', sources_dir]
)


# install completion autoloaders

# bash
install_data(
    autocompletions_dir / 'bash_autocomplete',
    rename: meson.project_name(),
    install_dir: 'share' / 'bash-completion' / 'completions'
)

# zsh
install_data(
    autocompletions_dir / 'zsh_autocomplete',
    rename: '_' + meson.project_name(),
    install_dir: 'share' / 'zsh' / 'site-functions'
)

# fish
install_data(
    autocompletions_dir / 'fish_autocomplete',
    rename: meson.project_name() + '.fish',
    install_dir: 'share' / 'fish' / 'completions'
)

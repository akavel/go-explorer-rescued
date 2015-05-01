" Copyright 2015 Gary Burd. All rights reserved.
" Use of this source code is governed by a BSD-style
" license that can be found in the LICENSE file.

function! s:getool()
    if executable('getool')
        return 'getool'
    endif

    let sep = '/'
    let list_sep = ':'
    for feature in ['win16', 'win32', 'win32unix', 'win64', 'win95']
        if (has(feature))
            let sep = '/'
            let list_sep = ';'
            break
        endif
    endfor

    let cmd = $GOBIN . sep . 'getool'
    if executable(cmd)
        return cmd
    endif

    for path in split($GOPATH, list_sep)
        let cmd = path . sep . 'bin' . sep . 'getool'
        if executable(cmd)
            return cmd
        endif
    endfor

    return 'getool'
endfunction

function! s:run(input, args)
    let cmd = s:getool()
    for arg in a:args
        let cmd = cmd . ' ' . shellescape(arg)
    endfor
    if a:input == ''
        return system(cmd)
    endif
    return system(cmd, a:input)
endfunction

" run returns output of running getool with arguments ... and stdin set to
" input.
function ge#tool#run(input, ...)
    return s:run(a:input, a:000)
endfunction

" runl is like run, but returns output as a list.
function ge#tool#runl(input, ...)
    return split(s:run(a:input, a:000), "\n", 1)
endfunction

" vim:ts=4:sw=4:et

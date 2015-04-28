" Copyright 2015 Gary Burd. All rights reserved.
" Use of this source code is governed by a BSD-style
" license that can be found in the LICENSE file.

function! s:import_text()
    let v = winsaveview()
    :0
    let p = searchpos('\v^(const|var|func|type)\s')
    call winrestview(v)
    let n = p[0] - 1
    if n <= 0
        return ' '
    endif
    return join(getline(1, n), "\n") . ' '
endfunction

function! ge#complete#complete(arg, line, pos)
    return split(system('getool complete ' . shellescape(a:arg) . ' ' . shellescape(a:line) . ' ' . shellescape(a:pos), s:import_text()))
endfunction

function! ge#complete#resolve(arg)
    return system('getool resolve ' . shellescape(a:arg), s:import_text())
endfunction

" vim:ts=4:sw=4:et

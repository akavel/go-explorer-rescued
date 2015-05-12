" Copyright 2015 Gary Burd. All rights reserved.
" Use of this source code is governed by a BSD-style
" license that can be found in the LICENSE file.

function! s:import_text() abort
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

function! ge#complete#complete(arg, line, pos) abort
    try
        return ge#tool#runl(s:import_text(), '-cwd', expand('%:p:h'), 'complete', a:arg, a:line, a:pos)
    catch /^go-explorer:/
        echom v:errmsg
        return a:arg
    endtry
endfunction

function! ge#complete#resolve(arg) abort
    return ge#tool#run(s:import_text(), '-cwd', expand('%:p:h'), 'resolve', a:arg)
endfunction

" vim:ts=4:sw=4:et

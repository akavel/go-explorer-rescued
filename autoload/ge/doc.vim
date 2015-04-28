" Copyright 2015 Gary Burd. All rights reserved.
" Use of this source code is governed by a BSD-style
" license that can be found in the LICENSE file.

" read loads a buffer with documentation, link and anchor data. This function
" is intended to be called from a BufReadCmd event.
function! ge#doc#read()
    setlocal noreadonly modifiable
    let b:strings = []
    let b:links = []
    let b:anchors = {}
    let cmd = 'getool doc ' . expand('%')
    let out = split(system(cmd), "\n", 1)

    if v:shell_error
        call append(0, out)
        setlocal buftype=nofile bufhidden=delete nobuflisted noswapfile nomodifiable
        return
    endif

    let index = 0
    while index < len(out)
        let line = out[index]
        let index = index + 1
        let m = matchlist(line, '\C\v^S (.*)')
        if len(m)
            let b:strings = add(b:strings, m[1])
            continue
        endif
        let m = matchlist(line, '\C\v^L ([0-9]+) ([0-9]+) ([0-9]+) ([-0-9]+)$')
        if len(m)
            call add(b:links, [m[1] + 0, m[2] + 0, m[3] + 0, m[4] + 0])
            continue
        endif
        let m = matchlist(line, '\C\v^A ([0-9]+) (\S+)$')
        if len(m)
            let b:anchors[m[2]] = m[1]
            continue
        endif
         if line ==# 'D'
            call append(0, out[index : -1])
            break
        endif
    endwhile
    setlocal buftype=nofile bufhidden=hide noswapfile nomodifiable readonly
    setfiletype godoc
    silent normal! gg
    nnoremap <buffer> <silent> <c-]> :call <SID>doc_jump()<CR>
    nnoremap <buffer> <silent> <c-t> :call <SID>doc_pop()<CR>
    nnoremap <buffer> <silent> ]] :call <SID>doc_next_section('')<CR>
    nnoremap <buffer> <silent> [[ :call <SID>doc_next_section('b')<CR>
endfunction

function! s:doc_link()
    let p = line('.') * 10000 + col('.')
    for t in b:links
        if p >= t[0]
            if p < t[1]
                return t
            endif
        else
            break
        endif
    endfor
    return []
endfunction

let s:doc_stack = []

function <SID>doc_jump()
    let link = s:doc_link()
    if len(link) == 0
        return
    endif
    let file = b:strings[link[2]]
    if link[3] >= 0
        let name = b:strings[link[3]]
    endif
    if file == "" || match(file, '^godoc://') == 0
        let s:doc_stack = add(s:doc_stack, [bufnr('%'), line('.'), col('.')])
    endif
    if file != ""
        execute 'edit ' . file
    endif
    let pos = 0
    if link[3] < 0
        let pos = -link[3]
    elseif exists('b:anchors') && name != ''
        let pos = get(b:anchors, name, 0)
    endif
    if pos
        exec pos / 10000
        exec 'normal! ' . (pos % 10000) . '|'
    endif
endfunction

function <SID>doc_pop()
    if len(s:doc_stack) == 0
        return
    endif
    let p = s:doc_stack[-1]
    let s:doc_stack = s:doc_stack[:-2]
    exec p[0] . 'b'
    exec p[1]
    exec 'normal! ' . p[2] . '|'
endfunction
  
function <SID>doc_next_section(dir)
    call search('\C\v^[^ )}]', 'W' . a:dir)
endfunction

" vim:ts=4:sw=4:et

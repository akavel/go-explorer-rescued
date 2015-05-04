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
    let out = ge#tool#runl('', 'doc', expand('%'))

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
            call add(b:links, [str2nr(m[1]), str2nr(m[2]), str2nr(m[3]), str2nr(m[4])])
            continue
        endif
        let m = matchlist(line, '\C\v^A ([0-9]+) (\S+)$')
        if len(m)
            let b:anchors[m[2]] = str2nr(m[1])
            continue
        endif
         if line ==# 'D'
            call append(0, out[index : -1])
            break
        endif
    endwhile
    setlocal foldmethod=syntax foldlevel=1 foldtext=ge#doc#foldtext()
    setlocal buftype=nofile bufhidden=hide noswapfile nomodifiable readonly tabstop=4
    setfiletype godoc
    silent 0
    nnoremap <buffer> <silent> <c-]> :call <SID>jump()<CR>
    nnoremap <buffer> <silent> <c-t> :call <SID>pop()<CR>
    nnoremap <buffer> <silent> ]] :call <SID>next_section('')<CR>
    nnoremap <buffer> <silent> [[ :call <SID>next_section('b')<CR>
    noremap <buffer> <silent> <2-LeftMouse> :call <SID>jump()<CR>
    autocmd! * <buffer>
    autocmd BufWinEnter <buffer> call s:update_highlight()
    autocmd BufWinLeave <buffer> call s:clear_highlight()
    autocmd CursorMoved <buffer> call s:update_highlight()
endfunction

function! s:update_highlight()
    let link = s:link()
    if exists('w:highlight_link') && w:highlight_link == link
        return
    endif
    let w:highlight_link = link
    if exists('w:highlight_match') && w:highlight_match
        call matchdelete(w:highlight_match)
        let w:highlight_match = 0
    endif
    if len(link) == 0
        return
    endif
    let w:highlight_match = matchadd('Underlined', '\%' . link[0] / 10000 . 'l\%' . link[0] % 10000 . 'c.\{' . (link[1] - link[0]) . '\}')
endfunction

function! s:clear_highlight()
    if exists('w:highlight_match') && w:highlight_match
        call matchdelete(w:highlight_match)
    endif
    let w:highlight_match = 0
    let w:highlight_link = []
endfunction

" link returns the link under the cursor or []
function! s:link()
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

let s:stack = []

function <SID>jump()
    let link = s:link()
    if len(link) == 0
        return
    endif
    let file = b:strings[link[2]]
    if link[3] >= 0
        let name = b:strings[link[3]]
    endif
    if file == "" || match(file, '^godoc://') == 0
        let s:stack = add(s:stack, [bufnr('%'), line('.'), col('.')])
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

function <SID>pop()
    if len(s:stack) == 0
        return
    endif
    let p = s:stack[-1]
    let s:stack = s:stack[:-2]
    exec p[0] . 'b'
    exec p[1]
    exec 'normal! ' . p[2] . '|'
endfunction

function <SID>next_section(dir)
    call search('\C\v^[^ \t)}]', 'W' . a:dir)
endfunction

function ge#doc#foldtext()
    let line = getline(v:foldstart)
    let m = matchlist(line, '\C\v^(var|const) ')
    if len(m)
        " show sorted list of constants and variables
        let start=10000 * v:foldstart
        let end = 10000 * v:foldend+1
        let ids = []
        for [id, pos] in items(b:anchors)
            if pos >= start && pos < end
                call add(ids, id)
            endif
        endfor
        sort(ids)
        return m[1] . ' ' . join(ids) . ' '
    endif
    if line[-2:] == ' {'
        " chop { following a struct or interface
        let line = line[:-3]
    endif
    return line . ' '
endfunction

" vim:ts=4:sw=4:et

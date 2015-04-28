" Copyright 2015 Gary Burd. All rights reserved.
" Use of this source code is governed by a BSD-style
" license that can be found in the LICENSE file.

if exists('b:current_syntax')
  finish
endif

syntax case match
syntax match godocHead '\C^[A-Z]\+$'

"syntax region godocDecl start='^\(package\|const\|var\|func\|type\) ' end='$' keepend contains=godocComment
"syntax region godocDecl start='^\(const\|var\) (' end='^)$' keepend contains=godocComment

syntax region godocDecl start='^\(package\|const\|var\|func\|type\) ' end='^$' contains=godocComment,godocParen,godocBrace
syntax region godocParen start='(' end=')' keepend contained contains=godocComment,godocParen,godocBrace
syntax region godocBrace start='{' end='}' keepend contained contains=godocComment,godocParen,godocBrace

syntax region godocDecl start='^type \S\+ \(interface\|struct\)' end='^}$' keepend contains=godocComment

syntax region godocComment start='/\*' end='\*/'  contained
syntax region godocComment start='//' end='$' contained

highlight link godocComment Comment
highlight link godocHead Constant
highlight link godocDecl Type
highlight link godocParen Type
highlight link godocBrace Type

let b:current_syntax = 'godoc'

" vim:ts=4 sts=2 sw=2:

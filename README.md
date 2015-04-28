Go Explorer is a Vim plugin for exploring Go code.

Go Explorer is a work in progress. It's usable, but there's much more to do.
Use the [Go Explorer issue tracker](https://github.com/garyburd/ge/issues) to
report bugs and request features.

## GeDoc

The GeDoc command shows documentation for the package specified by `spec`. 

    :GeDoc spec 

If `spec` starts with '/', then the remainder of `spec` is taken as a package
import path. Otherwise, `spec` is taken as the name of a package imported in
the current file. The GeDoc command supports command completion.

In the documentation viewer, use \<c-]> to jump to source code or
documentation.  If the identifier under the cursor is the name of a
declaration, then \<c-]> jumps to the source code for the declaration. If the
identifier under the cursor is a type, then \<c-]> jumps to the documentation
for the type. \<C-t> jumps back. Use \]] and \[\[ to move forward and back
through declarations in the documentation.

Documentation pages can be opened directly using the godoc:// prefix:

    :edit godoc://net/http

## Installation Instructions

To install this plugin with Pathogen, use:

     git clone https://github.com/garyburd/ge.git ~/.vim/bundle/ge

The plugin requires a Go helper program:

     go get github.com/garyburd/getool

The plugin and the getool program are tightly coupled. Update both at the
same time. 

## Other plugins

Go Explorder is compatible with [vim-go](https://github.com/fatih/vim-go).

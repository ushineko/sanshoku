/*
Package all lists every driver in the module, for a program that wants all of
them. There is no registry; a program that wants two drivers names two. It
opens nothing itself; the drivers it lists touch the kernel interfaces their
packages document.
*/
package all

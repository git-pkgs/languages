:- module(family, [ancestor/2]).
:- use_module(library(lists)).
ancestor(X,Y) :- parent(X,Y).

{ ... }: { home.packages = [ (import <nixpkgs> { }).hello ]; }

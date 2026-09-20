# The takeover edit and the resilience checks: a plain package addition
# that builds in well under a minute on a warm host.
{ pkgs, ... }: { home.packages = [ pkgs.cowsay ]; }

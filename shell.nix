{ pkgs ? import <nixpkgs> {} }:
with pkgs;
stdenv.mkDerivation {
  name = "irc-idler";
  buildInputs = [
    go_1_7
    sqlite
  ];
}

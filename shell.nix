{
  mkShell,
  pkgs,
  lib,
}:
mkShell {
  packages = with pkgs; [
    go
    alejandra
  ];

  shellHook = '''';
}

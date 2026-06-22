{
  description = "golinks";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/26.05";
  };

  outputs = {
    self,
    nixpkgs,
    ...
  } @ inputs: let
    inherit (nixpkgs) lib legacyPackages;

    platforms = ["x86_64-linux" "aarch64-linux" "aarch64-darwin"];

    forAllPlatforms = f:
      lib.genAttrs platforms (s: let
        pkgs = legacyPackages.${s};
      in
        f pkgs);
  in {
    formatter = forAllPlatforms (pkgs: pkgs.alejandra);

    devShells = forAllPlatforms (pkgs: let
      shell = pkgs.callPackage ./shell.nix {};
    in {
      default = shell;
    });
  };
}

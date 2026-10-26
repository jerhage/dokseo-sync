{
  description = "Sync server for Dokseo";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs =
    { self, nixpkgs }:
    let
      inherit (nixpkgs) lib;

      forAllSystems = lib.genAttrs [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
    in
    {
      packages = forAllSystems (system: rec {
        dokseo-sync = nixpkgs.legacyPackages.${system}.callPackage ./nix/package.nix { };
        default = dokseo-sync;
      });

      overlays.default = final: _prev: {
        dokseo-sync = final.callPackage ./nix/package.nix { };
      };

      # Defaults services.dokseo-sync.package to this flake's build, so a
      # consumer needs no overlay.
      nixosModules.dokseo-sync =
        { pkgs, ... }:
        {
          imports = [ ./nix/module.nix ];
          services.dokseo-sync.package =
            lib.mkDefault
              self.packages.${pkgs.stdenv.hostPlatform.system}.dokseo-sync;
        };
      nixosModules.default = self.nixosModules.dokseo-sync;

      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = [
              pkgs.go
              pkgs.gopls
            ];
          };
        }
      );

      formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.nixfmt-tree);
    };
}

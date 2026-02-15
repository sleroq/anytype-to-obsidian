{
  description = "Convert Anytype JSON export into Obsidian artifacts";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system);
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          version =
            if self ? shortRev then
              "unstable-${self.shortRev}"
            else
              "unstable";
        in
        {
          anytype-to-obsidian = pkgs.callPackage ./package.nix {
            inherit version;
          };
          default = self.packages.${system}.anytype-to-obsidian;
        }
      );

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/anytype-to-obsidian";
        };
      });
    };
}

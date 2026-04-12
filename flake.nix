{
  description = "Deej - hardware volume mixer for Linux";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.buildGoModule rec {
            pname = "deej";
            version = "1.0.0"; # Using a placeholder version since there's no explicit version

            src = ./.;

            vendorHash = "sha256-9g8AugKTVkT4cucMzcBS/vJk7lukzvS6jKyKMqEe2io=";

            # The main package is actually in pkg/deej/cmd directory
            subPackages = [ "pkg/deej/cmd" ];

            # Add ldflags to match the build script
            ldflags = [
              "-s"
              "-w"
              "-X main.versionTag=${version}"
              "-X main.buildType=release"
            ];

            # Rename the binary to have a more standard name
            postInstall = ''
              mv $out/bin/cmd $out/bin/deej
            '';

            meta = with pkgs.lib; {
              description = "Set app volumes with real sliders - a hardware volume mixer for Linux";
              homepage = "https://github.com/TheScabbage/deej-linux";
              license = licenses.mit;
              maintainers = [ ];
              platforms = platforms.linux;
            };
          };
        });

      overlays.default = final: prev: {
        deej = self.packages.${prev.system}.default;
      };

      nixosModules.default = import ./nix/module.nix;

      home-managerModules.default = import ./nix/module.nix;

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.mkShell {
            buildInputs = [
              pkgs.go
              pkgs.pkg-config
              pkgs.pulseaudio
              pkgs.playerctl
              pkgs.git
            ];
          };
        });
    };
}
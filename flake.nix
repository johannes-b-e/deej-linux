{
  description = "Development environment for deej";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }: {
    devShells.x86_64-linux.default = nixpkgs.legacyPackages.x86_64-linux.mkShell {
      buildInputs = [
        nixpkgs.legacyPackages.x86_64-linux.go
        nixpkgs.legacyPackages.x86_64-linux.pkg-config
        nixpkgs.legacyPackages.x86_64-linux.pulseaudio
        nixpkgs.legacyPackages.x86_64-linux.playerctl
        nixpkgs.legacyPackages.x86_64-linux.git
      ];
    };
  };
}
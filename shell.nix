with import <nixpkgs> {};
stdenv.mkDerivation {
  name = "deej";
  nativeBuildInputs = [ pkg-config ];
  buildInputs = [
    go
    pkg-config
    pulseaudio
    playerctl
    git
  ];
}

{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.deej;

  configFile = pkgs.writeText "deej-config.yaml" ''
    # process names are case-insensitive
    slider_mapping: ${builtins.toJSON cfg.sliderMapping}

    invert_sliders: ${boolToString cfg.invertSliders}

    com_port: ${cfg.serialPort}
    baud_rate: ${toString cfg.baudRate}

    noise_reduction: ${toString cfg.noiseReduction}
  '';

in
{
  options.services.deej = {
    enable = mkEnableOption "deej volume control service";

    sliderMapping = mkOption {
      type = types.attrsOf (types.either types.str (types.listOf types.str));
      default = {
        "0" = "master";
        "1" = [ "firefox" ];
        "2" = "deej.unmapped";
        "3" = [ "Chromium" "electron" ];
        "4" = [ "WEBRTC VoiceEngine" ".Discord-wrapped" ];
        "5" = "mic";
      };
      description = "Mapping of slider numbers to applications or 'master'/'mic'";
    };

    invertSliders = mkOption {
      type = types.bool;
      default = false;
      description = "Whether to invert slider values";
    };

    serialPort = mkOption {
      type = types.str;
      default = "/dev/ttyUSB0";
      description = "Serial port for the deej device";
    };

    baudRate = mkOption {
      type = types.int;
      default = 9600;
      description = "Baud rate for serial communication";
    };

    noiseReduction = mkOption {
      type = types.float;
      default = 0.025;
      description = "Noise reduction value";
    };

    verbose = mkOption {
      type = types.bool;
      default = true;
      description = "Enable verbose logging";
    };
    useDevBuild = mkOption {
      type = types.bool;
      default = false;
      description = "Use the dev build of deej (built with buildType=dev) instead of the release package";
    };
  };

  config = mkIf cfg.enable {
    # Install the binary (choose dev build if requested)
    home.packages = [ (if cfg.useDevBuild then pkgs.deejDev else pkgs.deej) ];

    # Create config file
    home.file.".config/deej/config.yaml".source = configFile;

    # Systemd user service
    systemd.user.services.deej = {
      Unit = {
        Description = "Deej volume control";
        After = [ "graphical-session.target" ];
        Wants = [ "graphical-session.target" ];
      };

      Service = {
        ExecStart = "${(if cfg.useDevBuild then pkgs.deejDev else pkgs.deej)}/bin/deej${optionalString cfg.verbose " -v"}";
        Restart = "always";
        RestartSec = 5;
        WorkingDirectory = "%h/.config/deej";
      };

      Install = {
        WantedBy = [ "default.target" ];
      };
    };

    # Enable user services
    systemd.user.startServices = "sd-switch";
  };
}
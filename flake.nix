{
  description = "Uncloud - Lightweight clustering and container orchestration tool";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config = {
            allowUnfree = false;
          };
        };

        version = if self ? rev then self.shortRev else "dev";

        commonAttrs = {
          pname = "uncloud";
          inherit version;
          src = builtins.path { path = ./.; name = "source"; };

          vendorHash = "sha256-vsvLc8kvALxnoWih1snlutOWKJs+O00C/ATqnFQLrlg=";

          buildInputs = [ ];

          ldflags = [
            "-s"
            "-w"
            "-X github.com/psviderski/uncloud/internal/version.version=${version}"
          ];

          meta = with pkgs.lib; {
            description = "Lightweight clustering and container orchestration tool";
            homepage = "https://uncloud.run";
            license = licenses.asl20;
            maintainers = [ ];
          };
        };

        uncloud = pkgs.buildGoModule (commonAttrs // {
          pname = "uncloud";
          subPackages = [ "cmd/uncloud" ];

          # CGO is required on macOS for fsevents (compose dependency)
          env.CGO_ENABLED = if pkgs.stdenv.isDarwin then "1" else "0";

          tags = [ ];
        });

        uncloudd = pkgs.buildGoModule (commonAttrs // {
          pname = "uncloudd";
          subPackages = [ "cmd/uncloudd" ];

          env.CGO_ENABLED = "0";

          meta = commonAttrs.meta // {
            platforms = pkgs.lib.platforms.linux;
            description = "Uncloud machine daemon";
          };
        });

        ucind = pkgs.buildGoModule (commonAttrs // {
          pname = "ucind";
          subPackages = [ "cmd/ucind" ];

          env.CGO_ENABLED = if pkgs.stdenv.isDarwin then "1" else "0";

          meta = commonAttrs.meta // {
            description = "Uncloud development cluster management tool";
          };
        });

        # Development shell with all tools from .mise.toml
        devShell = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            
            # Protobuf tools
            protobuf  # protoc 27.x or latest
            protoc-gen-go
            protoc-gen-go-grpc
            
            # Linting and formatting
            golangci-lint
            
            # Note: mockery is not in nixpkgs, install via: go install github.com/vektra/mockery/v2@latest
            
            # Node.js for documentation website
            nodejs_22
            
            # Additional development tools
            go-tools  # goimports, gopls, etc.
            git
            gnumake
          ];

          shellHook = ''
            echo "🌩️  Uncloud Development Environment"
            echo ""
            echo "Available commands:"
            echo "  go version:              $(go version)"
            echo "  protoc version:          $(protoc --version)"
            echo "  golangci-lint version:   $(golangci-lint version --short)"
            echo "  node version:            $(node --version)"
            echo ""
            echo "Build commands:"
            echo "  make                     - Show available make targets"
            echo "  go build ./cmd/uncloud   - Build CLI"
            echo "  go build ./cmd/uncloudd  - Build daemon"
            echo "  go build ./cmd/ucind     - Build dev cluster tool"
            echo ""
            echo "Nix commands:"
            echo "  nix build .#uncloud      - Build CLI with Nix"
            echo "  nix build .#uncloudd     - Build daemon with Nix (Linux only)"
            echo "  nix build .#ucind        - Build ucind with Nix"
            echo "  nix run .#uncloud        - Run CLI directly"
            echo ""
            echo "Note: Install mockery with: go install github.com/vektra/mockery/v2@latest"
            echo ""
          '';

          GOPATH = "${builtins.toString ./.}/.go";
          GO111MODULE = "on";
        };

      in
      {
        packages = {
          default = uncloud;
          inherit uncloud ucind;
        } // pkgs.lib.optionalAttrs pkgs.stdenv.isLinux {
          # uncloudd is only available on Linux
          inherit uncloudd;
        };

        # Apps for easy running with `nix run`
        apps = {
          default = {
            type = "app";
            program = "${uncloud}/bin/uncloud";
          };
          uncloud = {
            type = "app";
            program = "${uncloud}/bin/uncloud";
          };
          ucind = {
            type = "app";
            program = "${ucind}/bin/ucind";
          };
        } // pkgs.lib.optionalAttrs pkgs.stdenv.isLinux {
          uncloudd = {
            type = "app";
            program = "${uncloudd}/bin/uncloudd";
          };
        };

        devShells.default = devShell;

        formatter = pkgs.nixpkgs-fmt;
      }
    );
}

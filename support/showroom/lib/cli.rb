# frozen_string_literal: true

require 'docopt_ng'
require_relative 'runner'
require_relative 'video'
require_relative 'gif'

module Showroom
  # Docopt owns help and argument validation; commands dispatch independently.
  class CLI
    USAGE = <<~HELP
      Generate Commando showcase screenshots and videos.

      Usage:
        showroom shot [SCENE]
        showroom vid [SCENE]
        showroom gif [SCENE]
        showroom (-h | --help)

      Commands:
        shot   Capture screenshots using the Go renderer.
        vid    Assemble existing screenshots; never runs the renderer.
        gif    Convert existing video masters using Gifski.

      Arguments:
        SCENE  Scene name without an extension [default: workspace].
               Captures: config/scenes/SCENE.rb. Videos: config/movies/SCENE.rb.

      Options:
        -h --help  Show this help.

      Settings: config/showroom.yml. Generated files: out/THEME/SCENE/.
    HELP

    def self.run(argv, root)
      args = DocoptNG.docopt(USAGE, argv: argv)
      scene = scene_name(args)
      dispatch(args, scene, root)
      0
    rescue DocoptNG::Exit => e
      puts e.message
      e.exit_code
    end

    def self.scene_name(args)
      scene = args['SCENE'] || 'workspace'
      raise ArgumentError, 'scene must be a name containing letters, digits, underscores or hyphens' unless
        scene.match?(/\A[\w-]+\z/)

      scene
    end

    def self.dispatch(args, scene, root)
      if args['shot']
        Runner.new(root).run("config/scenes/#{scene}.rb")
      elsif args['vid']
        Video.new(root).run(scene)
      else
        Gif.new(root).run(scene)
      end
    end
  end
end

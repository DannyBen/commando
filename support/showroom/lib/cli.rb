# frozen_string_literal: true

require 'docopt_ng'
require_relative 'runner'
require_relative 'video'
require_relative 'gif'
require_relative 'publish'

module Showroom
  # Docopt owns help and argument validation; commands dispatch independently.
  class CLI
    USAGE = <<~HELP
      Generate Commando showcase screenshots and videos.

      Usage:
        showroom all
        showroom shot [SCENE]
        showroom vid [SCENE]
        showroom gif [SCENE]
        showroom publish
        showroom (-h | --help)

      Commands:
        all    Regenerate all site screenshots and videos, then copy to site assets.
        shot   Capture screenshots using the Go renderer.
        vid    Assemble existing screenshots; never runs the renderer.
        gif    Convert existing video masters using Gifski.
        publish  Copy current showcase media to the site.

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
      if args['all']
        all(root)
      elsif args['shot']
        Runner.new(root).run("config/scenes/#{scene}.rb")
      elsif args['vid']
        Video.new(root).run(scene)
      elsif args['publish']
        Publish.new(root).run
      else
        Gif.new(root).run(scene)
      end
    end

    def self.all(root)
      runner = Runner.new(root)
      runner.run('config/scenes/overview.rb')
      video = Video.new(root)
      Publish::POSTERS.each_key do |scene|
        runner.run("config/scenes/#{scene}.rb")
        video.run(scene)
      end
      Publish.new(root).run
    end
  end
end

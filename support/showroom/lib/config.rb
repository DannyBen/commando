# frozen_string_literal: true

require 'yaml'
require 'time'
require_relative 'workspace'

module Showroom
  # Loads capture settings and checks their references into the workspace.
  class Config
    attr_reader :output, :capture, :panels, :active_panel, :folders, :video, :gif

    def initialize(path)
      data = YAML.safe_load_file(path)
      base = File.dirname(path)
      @output = File.expand_path(data.fetch('output'), base)
      @capture = data.fetch('capture')
      @video = data.fetch('video')
      @gif = data.fetch('gif')
      load_panels(data)
      validate_capture
      @folders = Workspace.new(File.expand_path(data.fetch('workspace'), base)).folders
      validate_panels
    end

    def payload
      { capture: capture, panels: panels, active_panel: active_panel, folders: folders }
    end

    private

    def load_panels(data)
      @panels = data.fetch('panels')
      @active_panel = data.fetch('active_panel', 1)
    end

    def validate_capture
      %w[width height].each do |key|
        value = capture.fetch(key)
        unless value.is_a?(Integer) && (320..4096).cover?(value)
          raise ArgumentError,
                "#{key} must be between 320 and 4096"
        end
      end
      validate_themes
      Time.iso8601(capture.fetch('start_time'))
    end

    def validate_themes
      themes = capture.fetch('themes')
      return if themes.is_a?(Array) && !themes.empty? && themes.uniq == themes && (themes - %w[light dark]).empty?

      raise ArgumentError, 'themes must contain light and/or dark, without duplicates'
    end

    def validate_panels
      raise ArgumentError, 'panels must be nonempty and active_panel must identify a panel' unless valid_active_panel?

      panels.each { |panel| validate_panel_entries(panel) }
    end

    def valid_active_panel?
      panels.is_a?(Array) && !panels.empty? && active_panel.is_a?(Integer) &&
        (1..panels.length).cover?(active_panel)
    end

    def validate_panel_entries(panel)
      names = folders.fetch(panel.fetch('folder')).map { |item| item.fetch(:name) }
      requested = panel.fetch('marked', []) + [panel['selected']].compact
      missing = requested - names
      raise ArgumentError, "unknown panel entries: #{missing.join(', ')}" unless missing.empty?
    end
  end
end

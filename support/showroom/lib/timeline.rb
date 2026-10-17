# frozen_string_literal: true

module Showroom
  # A presentation timeline references named PNGs, without executing UI actions.
  class Timeline
    attr_reader :shots, :fps

    def initialize(path, fps)
      raise ArgumentError, 'video fps must be an integer from 1 to 60' unless
        fps.is_a?(Integer) && (1..60).cover?(fps)

      @fps = fps
      @shots = []
      instance_eval(File.read(path), path, 1)
      raise ArgumentError, 'movie needs at least one shot' if shots.empty?
    end

    def shot(name, duration:)
      raise ArgumentError, 'shot names must contain only letters, digits, underscores or hyphens' unless
        name.is_a?(String) && name.match?(/\A[\w-]+\z/)

      validate_duration(duration)
      shots << { file: "#{name}.png", frames: (duration * fps).round }
    end

    def frames
      shots.sum { |item| item.fetch(:frames) }
    end

    private

    def validate_duration(duration)
      return if duration.is_a?(Numeric) && duration.finite? && duration.positive? && (duration * fps).round.positive?

      raise ArgumentError, 'shot duration must be finite, positive and at least one frame'
    end
  end
end

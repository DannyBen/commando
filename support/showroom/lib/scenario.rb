# frozen_string_literal: true

module Showroom
  # Each DSL call emits an ordered instruction; only screenshot captures a frame.
  class Scenario
    attr_reader :steps, :theme, :dimensions

    def initialize(path, theme: nil)
      @theme = theme
      @steps = []
      @dimensions = {}
      @screenshots = []
      instance_eval(File.read(path), path, 1)
      raise ArgumentError, 'scene needs at least one screenshot' if @screenshots.empty?
    end

    # Canvas dimensions apply to the whole scene, independently of defaults.
    def size(width:, height:)
      raise ArgumentError, 'size must precede scene actions' unless steps.empty?

      canvas = { 'width' => width, 'height' => height }
      canvas.each do |name, value|
        raise ArgumentError, "#{name} must be an integer from 320 to 4096" unless
          value.is_a?(Integer) && (320..4096).cover?(value)
      end
      dimensions.merge!(canvas)
    end

    def press(key, times: 1)
      raise ArgumentError, 'times must be a positive integer' unless times.is_a?(Integer) && times.positive?

      times.times { steps << { key: key } }
    end

    # Scale physical pixels and Gio metrics together, preserving logical layout.
    def density(scale)
      raise ArgumentError, 'density must precede scene actions' unless steps.empty?
      raise ArgumentError, 'density must be an integer from 1 to 3' unless
        scale.is_a?(Integer) && (1..3).cover?(scale)

      dimensions['scale'] = scale
    end

    def type(text)
      steps << { text: text }
    end

    # Milliseconds of UI time, without sleeping in real time.
    def wait(milliseconds)
      unless milliseconds.is_a?(Integer) && milliseconds >= 0
        raise ArgumentError, 'wait requires nonnegative integer milliseconds'
      end

      steps << { wait_ms: milliseconds }
    end

    def screenshot(name)
      unless name.is_a?(String) && name.match?(/\A[\w-]+\z/) && !@screenshots.include?(name)
        raise ArgumentError, 'screenshot names must be unique and contain only letters, digits, underscores or hyphens'
      end

      @screenshots << name
      steps << { file: "#{name}.png" }
    end
  end
end

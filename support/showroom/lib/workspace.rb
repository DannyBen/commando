# frozen_string_literal: true

require 'yaml'
require 'time'

module Showroom
  # Expands a directory tree into separate virtual folder listings.
  class Workspace
    attr_reader :folders

    def initialize(path)
      data = YAML.safe_load_file(path)
      @defaults = data.fetch('defaults', {})
      @folders = {}
      data.fetch('roots').each { |folder, tree| add_folder(folder, tree) }
    end

    private

    def add_folder(path, tree)
      raise ArgumentError, "duplicate folder: #{path}" if folders.key?(path)

      entries = directory_entries(path, tree.fetch('directories', {}))
      entries.concat(tree.fetch('files', []).map { |file| metadata(file.fetch('name'), file) })
      validate_duplicates(path, entries)

      folders[path] = entries
    end

    def directory_entries(path, directories)
      directories.map do |name, child|
        validate_name(name)
        add_folder("#{path}\\#{name}", child)
        metadata(name, {}).merge(directory: true)
      end
    end

    def validate_duplicates(path, entries)
      names = entries.map { |item| item.fetch(:name).downcase }
      raise ArgumentError, "duplicate entries in #{path}" unless names.uniq == names
    end

    def metadata(name, file)
      validate_name(name)
      size = file.fetch('size', 0)
      raise ArgumentError, "invalid size for #{name}" unless size.is_a?(Integer) && size >= 0

      result = { name: name, bytes: size }
      %w[modified created].each do |key|
        date = file.fetch(key) { @defaults[key] }
        result[key] = Time.iso8601(date).utc.iso8601 if date
      end
      result
    end

    def validate_name(name)
      return if name.is_a?(String) && !name.empty? && !%w[. ..].include?(name) && !name.match?(%r{[\\/:]})

      raise ArgumentError, "invalid entry name: #{name.inspect}"
    end
  end
end

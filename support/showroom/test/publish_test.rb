# frozen_string_literal: true

require 'minitest/autorun'
require 'tmpdir'
require 'fileutils'
require 'yaml'
require_relative '../lib/publish'

class PublishTest < Minitest::Test
  ROOT = File.expand_path('..', __dir__)

  def setup
    @base = Dir.mktmpdir('showroom-publish-test')
    @root = File.join(@base, 'support/showroom')
    FileUtils.mkdir_p(File.join(@root, 'config'))
    config = YAML.safe_load_file(File.join(ROOT, 'config/showroom.yml'))
    config['workspace'] = File.join(ROOT, 'config/workspace.yml')
    config['capture']['themes'] = ['light']
    File.write(File.join(@root, 'config/showroom.yml'), YAML.dump(config))
    @output = File.join(@root, 'out/light')
    @assets = File.join(@base, 'site/assets')
  end

  def teardown
    FileUtils.remove_entry(@base)
  end

  def test_missing_source_does_not_publish_partial_assets
    FileUtils.mkdir_p(File.join(@output, 'overview'))
    File.write(File.join(@output, 'overview/overview.png'), 'placeholder')

    error = assert_raises(ArgumentError) { Showroom::Publish.new(@root).run }
    assert_match(/missing showcase asset/, error.message)
    refute Dir.exist?(@assets)
  end

  def test_publishes_all_mapped_assets
    files = { 'overview/overview.png' => 'overview' }
    Showroom::Publish::POSTERS.each do |scene, frame|
      files["#{scene}/preview.mp4"] = scene
      files["#{scene}/#{frame}.png"] = :image
    end
    files.each do |name, content|
      path = File.join(@output, name)
      FileUtils.mkdir_p(File.dirname(path))
      if content == :image
        raise 'ffmpeg failed' unless system('ffmpeg', '-nostdin', '-hide_banner', '-loglevel', 'error', '-y',
                                           '-f', 'lavfi', '-i', 'color=c=red:s=4x4', '-frames:v', '1', path)
      else
        File.write(path, content)
      end
    end

    Showroom::Publish.new(@root).run

    assert_equal 'overview', File.read(File.join(@assets, 'overview-light.png'))
    Showroom::Publish::POSTERS.each_key do |scene|
      assert_equal scene, File.read(File.join(@assets, "#{scene}-light.mp4"))
      assert File.size?(File.join(@assets, "#{scene}-light.jpg"))
    end
  end
end

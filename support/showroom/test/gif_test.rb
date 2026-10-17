# frozen_string_literal: true

require 'minitest/autorun'
require 'tmpdir'
require 'fileutils'
require 'open3'
require 'yaml'
require 'json'

# Real FFmpeg/Gifski conversion with no Go executable on the command's PATH.
class GifTest < Minitest::Test
  ROOT = File.expand_path('..', __dir__)

  def setup
    @root = Dir.mktmpdir('showroom-gif-test')
    @output = File.join(@root, 'out/light/workspace')
    FileUtils.mkdir_p([@output, File.join(@root, 'config'), File.join(@root, 'bin')])
    write_config
    %w[ffmpeg ffprobe gifski].each { |name| File.symlink(executable(name), File.join(@root, 'bin', name)) }
  end

  def teardown
    FileUtils.remove_entry(@root)
  end

  def executable(name)
    ENV.fetch('PATH').split(File::PATH_SEPARATOR).map { |dir| File.join(dir, name) }.find do |path|
      File.executable?(path)
    end || raise("#{name} is required for GIF integration tests")
  end

  def write_config
    data = YAML.safe_load_file(File.join(ROOT, 'config/showroom.yml'))
    data['workspace'] = File.join(ROOT, 'config/workspace.yml')
    data['capture']['themes'] = ['light']
    File.write(File.join(@root, 'config/showroom.yml'), YAML.dump(data))
  end

  def cli
    env = { 'PATH' => File.join(@root, 'bin') }
    code = 'exit Showroom::CLI.run(ARGV.drop(1), ARGV.first)'
    Open3.capture3(env, RbConfig.ruby, '-r', File.join(ROOT, 'lib/cli'), '-e', code, @root, 'gif')
  end

  def create_master(width = 32, height = 24)
    _stdout, stderr, status = Open3.capture3(executable('ffmpeg'), '-v', 'error', '-f', 'lavfi',
                                             '-i', "color=red:size=#{width}x#{height}:rate=10", '-frames:v', '5',
                                             '-c:v', 'ffv1', File.join(@output, 'master.mkv'))
    assert status.success?, stderr
  end

  def test_gif_command_uses_existing_master_without_go
    create_master
    stdout, stderr, status = cli
    assert status.success?, stderr
    refute_includes stdout, 'renderer'
    assert_gif
  end

  def test_gif_preserves_a_master_wider_than_the_logical_capture_width
    create_master(1920, 1080)
    _stdout, stderr, status = cli
    assert status.success?, stderr
    assert_gif(1920, 1080)
  end

  def assert_gif(width = 32, height = 24)
    json, status = Open3.capture2(executable('ffprobe'), '-v', 'error', '-show_streams', '-show_format',
                                  '-of', 'json', File.join(@output, 'preview.gif'))
    assert status.success?
    data = JSON.parse(json)
    assert_equal ['gif', width, height], data.fetch('streams').first.values_at('codec_name', 'width', 'height')
    assert_in_delta 0.5, data.fetch('format').fetch('duration').to_f, 0.01
  end

  def test_missing_master_explains_the_required_step
    _stdout, stderr, status = cli
    refute status.success?
    assert_includes stderr, 'showroom vid workspace first'
    refute File.exist?(File.join(@output, 'preview.gif'))
  end
end

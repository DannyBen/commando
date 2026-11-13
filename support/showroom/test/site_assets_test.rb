# frozen_string_literal: true

require 'minitest/autorun'
require 'tmpdir'
require 'fileutils'
require_relative '../../site_assets'

class SiteAssetsTest < Minitest::Test
  def setup
    @root = Dir.mktmpdir('commando-site-assets-test')
    FileUtils.mkdir_p(File.join(@root, 'assets'))
    %w[features.js site.js style.css favicon.ico assets/logo-light.svg assets/logo-dark.svg assets/movie.mp4 assets/social-mark.png].each do |path|
      File.write(File.join(@root, path), "content of #{path}")
    end
    @index = File.join(@root, 'index.html')
    File.write(@index, <<~HTML)
      <script id="asset-versions" type="application/json">{}</script>
      <script src="features.js"></script><script src="site.js"></script>
      <link href="style.css?mode=full&amp;v=old#sheet"><link href="favicon.ico">
      <img src="assets/logo-light.svg"><video data-dark-src="assets/movie.mp4"></video>
      <meta content="https://commando.sector-seven.com/assets/social-mark.png">
      <script src="https://example.com/site.js"></script>
    HTML
  end

  def teardown
    FileUtils.remove_entry(@root)
  end

  def test_versions_static_dynamic_and_social_assets_without_renaming_files
    files = Dir.glob('**/*', base: @root).sort
    versions = SiteAssets.new(@root).run
    html = File.read(@index)
    assert_equal files, Dir.glob('**/*', base: @root).sort
    %w[features.js site.js favicon.ico assets/logo-light.svg assets/movie.mp4].each do |path|
      assert_includes html, "#{path}?v=#{versions.fetch(path)}"
    end
    assert_includes html, "https://commando.sector-seven.com/assets/social-mark.png?v=#{versions.fetch('assets/social-mark.png')}"
    assert_includes html, 'src="https://example.com/site.js"'
    manifest = JSON.parse(html.match(/id="asset-versions"[^>]*>(.*?)<\/script>/m)[1])
    assert_equal versions, manifest
    assert_equal Digest::SHA256.hexdigest('content of assets/logo-dark.svg')[0, 12], manifest.fetch('assets/logo-dark.svg')
  end

  def test_repeated_runs_are_identical_and_preserve_other_query_parameters
    versions = SiteAssets.new(@root).run
    html = File.read(@index)
    assert_includes html, "style.css?mode=full&amp;v=#{versions.fetch('style.css')}#sheet"
    timestamp = Time.at(1000)
    File.utime(timestamp, timestamp, @index)
    assert_equal versions, SiteAssets.new(@root).run
    assert_equal html, File.read(@index)
    assert_equal timestamp, File.mtime(@index)
  end

  def test_only_changed_files_get_new_versions
    before = SiteAssets.new(@root).run
    File.write(File.join(@root, 'features.js'), 'updated catalogue')
    after = SiteAssets.new(@root).run
    refute_equal before.fetch('features.js'), after.fetch('features.js')
    assert_equal before.reject { |path, _| path == 'features.js' }, after.reject { |path, _| path == 'features.js' }
    assert_includes File.read(@index), "features.js?v=#{after.fetch('features.js')}"
    refute_includes File.read(@index), before.fetch('features.js')
  end

  def test_missing_manifest_fails_without_rewriting_html
    File.write(@index, '<script src="features.js"></script>')
    assert_raises(ArgumentError) { SiteAssets.new(@root).run }
    assert_equal '<script src="features.js"></script>', File.read(@index)
  end
end

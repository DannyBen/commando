# frozen_string_literal: true

module Showroom
  # Keeps long-running compiler and encoder calls visible in the console.
  module Progress
    private

    def progress(label)
      started = Process.clock_gettime(Process::CLOCK_MONOTONIC)
      puts "#{label}…"
      heartbeat = progress_heartbeat(label, started)
      yield
      report_progress(label, started, 'finished')
    ensure
      heartbeat&.kill
      heartbeat&.join
    end

    def progress_heartbeat(label, started)
      Thread.new do
        loop do
          sleep 5
          report_progress(label, started, '— still working')
        end
      end
    end

    def report_progress(label, started, status)
      elapsed = Process.clock_gettime(Process::CLOCK_MONOTONIC) - started
      puts format('%<label>s %<status>s (%<elapsed>.1fs)', label: label, status: status, elapsed: elapsed)
    end
  end
end

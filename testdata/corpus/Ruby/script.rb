#!/usr/bin/env ruby
# frozen_string_literal: true
require 'json'

def load(path)
  JSON.parse(File.read(path))
end

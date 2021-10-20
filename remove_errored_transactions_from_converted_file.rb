require 'json'
require 'optparse'

@options = {}
OptionParser.new do |opts|
  opts.on("-c", "--converted-file C", "The converted file from the payout_json_conversion.py tool") do |file|
    @options[:converted_file] = file
  end
  opts.on("-e", "--error-file E", "The error file from running upload") do |file|
    @options[:error_file] = file
  end
end.parse!

fail "Converted File not provided" unless @options[:converted_file]
fail "Error File not provided" unless @options[:error_file]

p @options

json_converted = JSON.parse(File.read(@options[:converted_file]))
json_error = JSON.parse(File.read(@options[:error_file]))

original_entries_to_keep = json_converted.select do |orig_entry|
    json_error.none? do |error_entry|
        orig_entry["owner"] == error_entry["owner"] &&
        orig_entry["publisher"] == error_entry["publisher"]
    end
end

fail "Too many or too few error entries removed!" if original_entries_to_keep.size != (json_converted.size - json_error.size)

puts "Found #{json_converted.size} original entries"
puts "Found #{json_error.size} error entries"

puts "Copying original file to backup"
`cp -n #{@options[:converted_file]} #{File.basename(@options[:converted_file], '.json') + 'original.json'}`

error_free_file = File.basename(@options[:converted_file], '.json') + '_without_error_entries.json'
puts "Writing out #{original_entries_to_keep.size} new error-free entries to #{error_free_file}"

File.open(error_free_file,"w") do |f|
    f.write(JSON.pretty_generate(original_entries_to_keep))
end

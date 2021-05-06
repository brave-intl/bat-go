import json
import argparse

parser = argparse.ArgumentParser(description='Convert ads payout file for settlement tool')
parser.add_argument('--input', type=str)

args = parser.parse_args()
filename = args.input

f = open(filename, 'r')
data = json.load(f)
provider ='bitflyer'

provider_exclusive_data = []
total_bat = 0

for transaction in data:
    if transaction['walletProvider'] != provider:
        continue
    bat = float(transaction['probi']) / 1E18
    if transaction['walletProvider'] == 'uphold':
        transaction['wallet_provider'] = 'uphold'
        transaction['bat'] = str(bat)
    elif transaction['walletProvider'] == 'bitflyer':
        transaction['wallet_provider'] = 'bitflyer'
        transaction['wallet_provider_id'] = "bitflyer#id:" + transaction['address']
        transaction['bat'] = str(round(bat, 8))

    total_bat += float(transaction['bat'])
    transaction['publisher'] = "wallet:" + transaction['address']

    # delete unused keys
    transaction.pop('walletProvider')
    transaction.pop('probi')
    transaction.pop('transactionId')

    provider_exclusive_data.append(transaction)

print(total_bat)
output_filename = provider + "_" + filename.split('.json')[0] + "_converted.json"
with open(output_filename, 'w') as outfile:
    json.dump(provider_exclusive_data, outfile, indent=4)

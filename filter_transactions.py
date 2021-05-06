import json
import argparse
from decimal import *

parser = argparse.ArgumentParser(description='Convert ads payout file for settlement tool')
parser.add_argument('--input', type=str)

args = parser.parse_args()
filename = args.input

f = open(filename, 'r')
data = json.load(f)
amount = 0

def split_cr(filename, provider, contributions, referrals):
    contributions_output_filename = filename.split('.json')[0] + f"-{provider}-contributions.json"
    with open(contributions_output_filename, 'w') as outfile:
        json.dump(contributions, outfile, indent=4)

    referrals_output_filename = filename.split('.json')[0] + f"-{provider}-referrals.json"
    with open(referrals_output_filename, 'w') as outfile:
        json.dump(referrals, outfile, indent=4)

def split_ads(filename, provider, ads):
    contributions_output_filename = filename.split('.json')[0] + f"-{provider}-{filename}.json"
    with open(contributions_output_filename, 'w') as outfile:
        json.dump(contributions, outfile, indent=4)

    referrals_output_filename = filename.split('.json')[0] + f"-{provider}-referrals.json"
    with open(referrals_output_filename, 'w') as outfile:
        json.dump(referrals, outfile, indent=4)

def convert_to_id_and_usd_amount(transactions):
    ids = []
    for transaction in transactions:
        bat = Decimal(transaction['bat'])
        usd = (Decimal(1.19) * bat).quantize(Decimal('.01'), rounding=ROUND_UP)
        ids.append(f"{transaction['owner'].split(':')[1]},{usd}")
    with open("publisher_paypal_ids.csv", 'w') as outfile:
        json.dump(ids, outfile, indent=4)

# this should be the default already
getcontext().prec = 28
contributions = []
referrals = []
gemini_transactions = []
contribution_amount = 0
referral_amount = 0
provider = "bitflyer"

ads_transactions = []
ads_amount = 0

for transaction in data:
    if transaction['wallet_provider_id'].startswith(provider) or transaction['walletProvider'].startswith(provider):
        if transaction['type'] == 'contribution':
            contributions.append(transaction)
            contribution_amount += Decimal(transaction['bat'])
        elif transaction['type'] == 'referral':
            referrals.append(transaction)
            referral_amount += Decimal(transaction['bat'])
        elif transaction['type'] == 'adsDirectDeposit':
            ads_transactions.append(transaction)
            ads_amount += Decimal(transaction['probi'])

# convert_to_id_and_usd_amount(gemini_transactions)
# split_cr(filename, provider, contributions, referrals)


# print("total contribution amount: " + str(contribution_amount))
# print("total referral amount: " + str(referral_amount))
# print("total: " + str(referral_amount + contribution_amount))
print('ads amount: ' + str(ads_amount / 1E18))

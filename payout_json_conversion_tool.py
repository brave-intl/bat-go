import json
import argparse
from os import listdir
from os.path import isfile, join
from decimal import *
import re

def split_contributions_referrals(filename, provider, contributions, referrals):
    contributions_output_filename = filename.split('.json')[0] + f"-{provider}-contributions.json"
    with open(contributions_output_filename, 'w') as outfile:
        json.dump(contributions, outfile, indent=4)

    referrals_output_filename = filename.split('.json')[0] + f"-{provider}-referrals.json"
    with open(referrals_output_filename, 'w') as outfile:
        json.dump(referrals, outfile, indent=4)

def partition(pred, iterable):
    trues = []
    falses = []
    for item in iterable:
        if pred(item):
            trues.append(item)
        else:
            falses.append(item)
    return trues, falses

def convert_publishers_file(filename, provider):
    f = open(filename, 'r')
    data = json.load(f)
    amount = 0

    # this should be the default already
    getcontext().prec = 28
    contributions = []
    referrals = []
    gemini_transactions = []
    contribution_amount = 0
    referral_amount = 0
    
    # Group the Bitflyer payouts to the same channel, summing amount due into 1 transaction
    bitflyer_txs, non_bitflyer_txs = partition(lambda x: x['wallet_provider_id'] and x['wallet_provider_id'].startswith('bitflyer'), data)
    # non_bitflyer_txs = (x for x in data if not x['wallet_provider_id'].startswith('bitflyer'))
    new_grouped_bitflyer_txs = []
    for transaction in bitflyer_txs:
        # Find the current publisher/destination in list
        found_bitflyer_tx = next((x for x in new_grouped_bitflyer_txs if (x['wallet_provider_id'] == transaction['wallet_provider_id'])), None)

        if found_bitflyer_tx:
            # If found, add our value to it, and go to next
            found_bitflyer_tx["bat"] = Decimal(found_bitflyer_tx['bat']) + Decimal(transaction['bat'])
        else:
            # If not found, add us to the array
            new_grouped_bitflyer_txs.append(transaction)


    data_with_adjusted_bitflyer = [*non_bitflyer_txs, *new_grouped_bitflyer_txs]
    # now do regular conversion
    for transaction in data_with_adjusted_bitflyer:
        channel_id = transaction["publisher"]
        if transaction['wallet_provider_id'] != None and transaction['wallet_provider_id'].startswith(provider):
            if transaction['type'] == 'contribution':
                contributions.append(transaction)
                contribution_amount += Decimal(transaction['bat'])
            elif transaction['type'] == 'referral':
                referrals.append(transaction)
                referral_amount += Decimal(transaction['bat'])
            if transaction['wallet_provider_id'].startswith('bitflyer'):
                bat = float(transaction['bat'])
                transaction['bat'] = str(round(bat, 2))

    split_contributions_referrals(filename, provider, contributions, referrals)

    print("total contribution amount: " + str(contribution_amount))
    print("total referral amount: " + str(referral_amount))
    print("total: " + str(referral_amount + contribution_amount))


def convert_ads_file(filename, provider):
    f = open(filename, 'r')
    data = json.load(f)

    provider_exclusive_data = []
    total_bat = 0
    suffix = 1
    suffix_name = "_converted_fixed_"
    for transaction in data:
        if transaction['walletProvider'] != provider:
            continue
        bat = float(transaction['probi']) / 1E18
        if transaction['walletProvider'] == 'uphold':
            card_id = transaction['publisher'].split(':')[1]
            transaction['wallet_provider'] = 'uphold'
            transaction['wallet_provider_id'] = "uphold#card:" + card_id
            transaction['bat'] = str(bat)
            transaction['owner'] = transaction['publisher']
        elif transaction['walletProvider'] == 'bitflyer':
            transaction['wallet_provider'] = 'bitflyer'
            transaction['wallet_provider_id'] = "bitflyer#id:" + transaction['address']
            transaction['bat'] = str(round(bat, 2))
            transaction['publisher'] = "wallet:" + transaction['address']
            transaction['owner'] = "wallet:" + transaction['address']
        elif transaction['walletProvider'] == 'gemini':
            card_id = transaction['publisher'].split(':')[1]
            transaction['wallet_provider'] = 'gemini'
            transaction['wallet_provider_id'] = "gemini#id:" + card_id
            transaction['bat'] = str(bat)
            transaction['publisher'] = "wallet:" + transaction['address']
            transaction['owner'] = "wallet:" + transaction['address']

        total_bat += float(transaction['bat'])
        transaction['payout_report_id'] = transaction['transactionId']

        # delete unused keys
        transaction.pop('walletProvider')
        transaction.pop('probi')
        transaction.pop('transactionId')

        provider_exclusive_data.append(transaction)

        # There is some odd limitation with > 100 transaction in a bulk, as the API call also hangs quite a bit
        if provider in ['bitflyer', 'gemini'] and len(provider_exclusive_data) > 10:
            output_filename = provider + "_" + filename.split('.json')[0] + suffix_name + str(suffix) + ".json"
            with open(output_filename, 'w') as outfile:
                json.dump(provider_exclusive_data, outfile, indent=4)
            suffix += 1
            provider_exclusive_data = []

    if provider in ['bitflyer', 'gemini']:
        output_filename = provider + "_" + filename.split('.json')[0] + suffix_name + str(suffix) + ".json"
        with open(output_filename, 'w') as outfile:
            json.dump(provider_exclusive_data, outfile, indent=4)
    elif provider in ['uphold'] :
        output_filename = provider + "_" + filename.split('.json')[0] + "_converted.json"
        with open(output_filename, 'w') as outfile:
            json.dump(provider_exclusive_data, outfile, indent=4)

    print(f"Total BAT: {total_bat} to {output_filename}")

def main():
    parser = argparse.ArgumentParser(description='Convert ads or publishers payout file for settlement tool')
    parser.add_argument('--input', type=str)
    parser.add_argument('--provider', type=str)
    parser.add_argument('--kind', type=str)

    args = parser.parse_args()
    filename = args.input
    provider = args.provider
    kind = args.kind

    if kind == 'ads':
        convert_ads_file(filename, provider)
    elif kind == 'publishers':
        convert_publishers_file(filename, provider)

if __name__ == "__main__":
    main()

#! /bin/bash
rm -rf eyeshade/migrations* eyeshade/seeds
cp -r $BAT_LEDGER/eyeshade/migrations ./eyeshade/ && \
rm eyeshade/migrations/current.js
mkdir -p eyeshade/migrations-2
mkdir -p eyeshade/seeds
for i in $(ls eyeshade/migrations); do
  for j in $(ls eyeshade/migrations/$i/); do
    if [[ -f eyeshade/migrations/$i/$j ]]
    then
      cp eyeshade/migrations/$i/$j eyeshade/migrations-2/$i.$j
    else
      for k in $(ls eyeshade/migrations/$i/$j); do
        cp eyeshade/migrations/$i/$j/$k eyeshade/seeds/$i\_$k
      done
    fi
  done
done
rm -rf eyeshade/migrations
mv eyeshade/migrations-2 eyeshade/migrations
